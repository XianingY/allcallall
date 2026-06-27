//go:build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"

	"github.com/allcallall/backend/internal/storage"
)

func TestBetaDependencies(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Run("mysql", func(t *testing.T) {
		dsn := requiredEnv(t, "INTEGRATION_MYSQL_DSN")
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			t.Fatalf("open mysql: %v", err)
		}
		defer db.Close()
		if err := db.PingContext(ctx); err != nil {
			t.Fatalf("ping mysql: %v", err)
		}
		if _, err := db.ExecContext(ctx, "CREATE TEMPORARY TABLE integration_probe (id BIGINT PRIMARY KEY)"); err != nil {
			t.Fatalf("create mysql probe table: %v", err)
		}
		if _, err := db.ExecContext(ctx, "INSERT INTO integration_probe (id) VALUES (1)"); err != nil {
			t.Fatalf("write mysql probe: %v", err)
		}
	})

	t.Run("redis", func(t *testing.T) {
		client := redis.NewClient(&redis.Options{Addr: requiredEnv(t, "INTEGRATION_REDIS_ADDR")})
		defer client.Close()
		if err := client.Ping(ctx).Err(); err != nil {
			t.Fatalf("ping redis: %v", err)
		}
		if err := client.Set(ctx, "allcallall:integration:probe", "ok", time.Minute).Err(); err != nil {
			t.Fatalf("write redis probe: %v", err)
		}
		if value, err := client.Get(ctx, "allcallall:integration:probe").Result(); err != nil || value != "ok" {
			t.Fatalf("read redis probe: value=%q err=%v", value, err)
		}
	})

	t.Run("minio", func(t *testing.T) { testMinIO(t, ctx) })
	t.Run("elasticsearch", func(t *testing.T) { testElasticsearch(t, ctx) })
}

func testMinIO(t *testing.T, ctx context.Context) {
	t.Helper()
	endpoint := requiredEnv(t, "INTEGRATION_MINIO_ENDPOINT")
	accessKey := requiredEnv(t, "INTEGRATION_MINIO_ACCESS_KEY")
	secretKey := requiredEnv(t, "INTEGRATION_MINIO_SECRET_KEY")
	bucket := "allcallall-integration"
	config, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		awsconfig.WithBaseEndpoint(endpoint),
	)
	if err != nil {
		t.Fatalf("load minio config: %v", err)
	}
	client := s3.NewFromConfig(config, func(options *s3.Options) { options.UsePathStyle = true })
	if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil && !strings.Contains(err.Error(), "BucketAlreadyOwnedByYou") {
		t.Fatalf("create minio bucket: %v", err)
	}
	recordings, err := storage.NewRecordingStorage(storage.Config{
		Driver: storage.DriverS3, S3Bucket: bucket, S3Region: "us-east-1", S3Endpoint: endpoint,
		S3AccessKeyID: accessKey, S3SecretKey: secretKey, S3ForcePath: true,
	})
	if err != nil {
		t.Fatalf("create S3 recording storage: %v", err)
	}
	path := filepath.Join(t.TempDir(), "probe.ogg")
	if err := os.WriteFile(path, []byte("integration-audio"), 0o600); err != nil {
		t.Fatalf("write probe file: %v", err)
	}
	object, err := recordings.SaveFile(ctx, path, "integration/probe.ogg", "audio/ogg")
	if err != nil {
		t.Fatalf("save minio recording: %v", err)
	}
	reader, err := recordings.Open(ctx, *object)
	if err != nil {
		t.Fatalf("open minio recording: %v", err)
	}
	content, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil || string(content) != "integration-audio" {
		t.Fatalf("read minio recording: content=%q err=%v", content, readErr)
	}
	if err := recordings.Delete(ctx, *object); err != nil {
		t.Fatalf("delete minio recording: %v", err)
	}
}

func testElasticsearch(t *testing.T, ctx context.Context) {
	t.Helper()
	baseURL := strings.TrimRight(requiredEnv(t, "INTEGRATION_ELASTICSEARCH_URL"), "/")
	requestJSON := func(method, path, body string, expected ...int) {
		request, err := http.NewRequestWithContext(ctx, method, baseURL+path, strings.NewReader(body))
		if err != nil {
			t.Fatalf("create elasticsearch request: %v", err)
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("request elasticsearch: %v", err)
		}
		defer response.Body.Close()
		for _, status := range expected {
			if response.StatusCode == status {
				return
			}
		}
		t.Fatalf("unexpected elasticsearch status: %d", response.StatusCode)
	}
	requestJSON(http.MethodPut, "/allcallall-integration", `{}`, http.StatusOK, http.StatusBadRequest)
	requestJSON(http.MethodPut, "/allcallall-integration/_doc/1?refresh=true", `{"text":"meeting transcript"}`, http.StatusCreated, http.StatusOK)
	requestJSON(http.MethodPost, "/allcallall-integration/_search", `{"query":{"match":{"text":"transcript"}}}`, http.StatusOK)
	requestJSON(http.MethodDelete, "/allcallall-integration", "", http.StatusOK)
}

func requiredEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatal(fmt.Sprintf("%s is required", name))
	}
	return value
}
