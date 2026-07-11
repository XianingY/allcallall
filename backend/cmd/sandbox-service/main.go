package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/mcpplatform"
	"github.com/allcallall/backend/internal/sandbox"
)

func main() {
	log := zerolog.New(os.Stdout).With().Timestamp().Str("service", "sandbox-control-plane").Logger()
	runner, err := sandbox.NewHTTPRunner(os.Getenv("SANDBOX_RUNNER_URL"))
	if err != nil {
		log.Fatal().Err(err).Msg("initialize sandbox runner")
	}
	service := sandbox.NewService(runner, sandbox.TrivyScanner{Binary: os.Getenv("TRIVY_BINARY")})
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
			writeSandboxError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	address := strings.TrimSpace(os.Getenv("SANDBOX_ADDR"))
	if address == "" {
		address = ":8092"
	}
	server := &http.Server{
		Addr:              address,
		Handler:           http.MaxBytesHandler(mux, 512*1024),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      40 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	rootContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
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
	case errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
		code = "SANDBOX_TIMEOUT"
	}
	writeJSON(w, status, map[string]string{"code": code, "error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
