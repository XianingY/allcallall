package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"github.com/allcallall/backend/internal/mcpplatform"
	"github.com/allcallall/backend/internal/sandbox"
)

func main() {
	log := zerolog.New(os.Stdout).With().Timestamp().Str("service", "sandbox-control-plane").Logger()
	capabilityVerifier, err := mcpplatform.NewSandboxCapabilityVerifierFromEnv()
	if err != nil {
		log.Fatal().Err(err).Msg("initialize sandbox capability verifier")
	}
	db, closeDB, err := openReceiptDB(strings.TrimSpace(os.Getenv("SANDBOX_DB_DSN")))
	if err != nil {
		log.Fatal().Err(err).Msg("initialize sandbox receipt database")
	}
	defer closeDB()
	runner, err := sandbox.NewHTTPRunner(os.Getenv("SANDBOX_RUNNER_URL"))
	if err != nil {
		log.Fatal().Err(err).Msg("initialize sandbox runner")
	}
	receiptStore := sandbox.NewReceiptStore(db)
	service := sandbox.NewService(runner, sandbox.TrivyScanner{Binary: os.Getenv("TRIVY_BINARY")}).
		WithReceiptStore(receiptStore)
	if envEnabled("SANDBOX_KUBERNETES_EXECUTION_ENABLED") {
		ociRunner, err := sandbox.NewInClusterKubernetesRunner(sandbox.KubernetesRunnerConfig{
			Namespace:        os.Getenv("SANDBOX_JOB_NAMESPACE"),
			RunnerImage:      os.Getenv("SANDBOX_JOB_RUNNER_IMAGE"),
			SupervisorImage:  os.Getenv("SANDBOX_JOB_SUPERVISOR_IMAGE"),
			RuntimeClass:     os.Getenv("SANDBOX_JOB_RUNTIME_CLASS"),
			ServiceAccount:   os.Getenv("SANDBOX_JOB_SERVICE_ACCOUNT"),
			AppName:          os.Getenv("SANDBOX_JOB_APP_NAME"),
			Instance:         os.Getenv("SANDBOX_JOB_INSTANCE"),
			OpenBaoAddress:   os.Getenv("OPENBAO_ADDR"),
			ImagePullSecrets: parseImagePullSecrets(os.Getenv("SANDBOX_JOB_IMAGE_PULL_SECRETS")),
			CPU:              os.Getenv("SANDBOX_JOB_CPU"),
			Memory:           os.Getenv("SANDBOX_JOB_MEMORY"),
			StartupTimeout:   envDuration("SANDBOX_JOB_STARTUP_TIMEOUT", 30*time.Second),
		})
		if err != nil {
			log.Fatal().Err(err).Msg("initialize isolated OCI Kubernetes runner")
		}
		service.WithOCIRunner(ociRunner)
	}
	handler := newHandler(service, capabilityVerifier)
	address := strings.TrimSpace(os.Getenv("SANDBOX_ADDR"))
	if address == "" {
		address = ":8092"
	}
	server := &http.Server{
		Addr:              address,
		Handler:           http.MaxBytesHandler(handler, 512*1024),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      40 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	rootContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go runReceiptCleanup(rootContext, receiptStore, log)
	go func() {
		log.Info().Str("address", address).Msg("sandbox control plane listening")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("sandbox control plane failed")
		}
	}()
	<-rootContext.Done()
	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		log.Error().Err(err).Msg("sandbox control plane shutdown failed")
	}
}

func envEnabled(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return fallback
	}
	return duration
}

func parseImagePullSecrets(value string) []string {
	var references []struct {
		Name string `json:"name"`
	}
	if strings.TrimSpace(value) == "" || json.Unmarshal([]byte(value), &references) != nil {
		return nil
	}
	names := make([]string, 0, len(references))
	for _, reference := range references {
		if name := strings.TrimSpace(reference.Name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func newHandler(service *sandbox.Service, capabilityVerifier *mcpplatform.SandboxCapabilityVerifier) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /internal/v1/installations/validate", func(w http.ResponseWriter, request *http.Request) {
		var input mcpplatform.ValidationRequest
		if !decodeRequest(w, request, &input) {
			return
		}
		result, err := service.Validate(request.Context(), input)
		if err != nil {
			writeSandboxError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("POST /internal/v1/executions", func(w http.ResponseWriter, request *http.Request) {
		var input mcpplatform.ExecutionRequest
		if !decodeRequest(w, request, &input) {
			return
		}
		result, err := service.Execute(request.Context(), input)
		if err != nil {
			if errors.Is(err, sandbox.ErrExecutionInProgress) {
				writeJSON(w, http.StatusAccepted, result)
				return
			}
			if errors.Is(err, sandbox.ErrExecutionConflict) {
				writeJSON(w, http.StatusConflict, map[string]any{
					"code":      "SANDBOX_EXECUTION_CONFLICT",
					"error":     err.Error(),
					"execution": result,
				})
				return
			}
			writeSandboxError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("GET /internal/v1/executions/{execution_id}", func(w http.ResponseWriter, request *http.Request) {
		result, err := service.LookupExecution(request.Context(), request.PathValue("execution_id"))
		if err != nil {
			if errors.Is(err, sandbox.ErrReceiptNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{
					"code":  "SANDBOX_EXECUTION_NOT_FOUND",
					"error": "sandbox execution receipt not found",
				})
				return
			}
			writeSandboxError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	return requireSandboxCapability(capabilityVerifier, mux)
}

func requireSandboxCapability(verifier *mcpplatform.SandboxCapabilityVerifier, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			next.ServeHTTP(w, request)
			return
		}
		token, err := mcpplatform.SandboxAuthorizationToken(request)
		if err != nil {
			writeSandboxAuthorizationError(w)
			return
		}
		digest, err := sandboxRequestDigest(request)
		if err != nil || verifier.Verify(token, request.Method, request.URL.EscapedPath(), digest) != nil {
			writeSandboxAuthorizationError(w)
			return
		}
		next.ServeHTTP(w, request)
	})
}

func sandboxRequestDigest(request *http.Request) (string, error) {
	switch {
	case request.Method == http.MethodPost && request.URL.Path == "/internal/v1/installations/validate":
		var input mcpplatform.ValidationRequest
		if err := decodeSandboxCapabilityBody(request, &input); err != nil {
			return "", err
		}
		return mcpplatform.ValidationAuthorizationRequestDigest(input)
	case request.Method == http.MethodPost && request.URL.Path == "/internal/v1/executions":
		var input mcpplatform.ExecutionRequest
		if err := decodeSandboxCapabilityBody(request, &input); err != nil {
			return "", err
		}
		return mcpplatform.ExecutionAuthorizationRequestDigest(input)
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/internal/v1/executions/"):
		executionID := strings.TrimPrefix(request.URL.Path, "/internal/v1/executions/")
		return mcpplatform.SandboxLookupRequestDigest(executionID)
	default:
		return "", mcpplatform.ErrInvalidSandboxCapability
	}
}

func decodeSandboxCapabilityBody(request *http.Request, output any) error {
	const requestLimit = 512 * 1024
	originalBody := request.Body
	body, err := io.ReadAll(io.LimitReader(originalBody, requestLimit+1))
	_ = originalBody.Close()
	if err != nil {
		return err
	}
	if len(body) > requestLimit {
		return fmt.Errorf("sandbox request exceeds %d bytes", requestLimit)
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("request body contains trailing JSON")
	}
	return nil
}

func writeSandboxAuthorizationError(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	writeJSON(w, http.StatusUnauthorized, map[string]string{
		"code":  "SANDBOX_CAPABILITY_DENIED",
		"error": "sandbox capability is missing or invalid",
	})
}

func decodeRequest(w http.ResponseWriter, request *http.Request, output any) bool {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "INVALID_REQUEST", "error": "invalid request body"})
		return false
	}
	return true
}

func writeSandboxError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	code := "SANDBOX_EXECUTION_FAILED"
	switch {
	case errors.Is(err, sandbox.ErrPrivateAddress):
		status = http.StatusForbidden
		code = "SANDBOX_NETWORK_DENIED"
	case errors.Is(err, sandbox.ErrImageRejected):
		status = http.StatusBadRequest
		code = "SANDBOX_IMAGE_REJECTED"
	case errors.Is(err, sandbox.ErrInvalidExecution):
		status = http.StatusBadRequest
		code = "SANDBOX_INVALID_EXECUTION"
	case errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
		code = "SANDBOX_TIMEOUT"
	case errors.Is(err, sandbox.ErrReceiptUnavailable):
		status = http.StatusServiceUnavailable
		code = "SANDBOX_RECEIPT_UNAVAILABLE"
	}
	writeJSON(w, status, map[string]string{"code": code, "error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func openReceiptDB(dsn string) (*gorm.DB, func(), error) {
	if dsn == "" {
		return nil, nil, fmt.Errorf("SANDBOX_DB_DSN is required")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
	})
	if err != nil {
		return nil, nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, err
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(10 * time.Minute)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	closeDB := func() {
		_ = sqlDB.Close()
	}
	return db, closeDB, nil
}

func runReceiptCleanup(ctx context.Context, store *sandbox.ReceiptStore, log zerolog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			deleted, err := store.DeleteExpired(cleanupCtx, now.UTC(), 500)
			cancel()
			if err != nil {
				log.Error().Err(err).Msg("clean expired sandbox receipts")
			} else if deleted > 0 {
				log.Info().Int64("deleted", deleted).Msg("cleaned expired sandbox receipts")
			}
		}
	}
}
