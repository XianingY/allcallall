package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Driver string

const (
	DriverLocal Driver = "local"
	DriverS3    Driver = "s3"
)

type Config struct {
	Driver        Driver
	LocalRoot     string
	S3Bucket      string
	S3Region      string
	S3Endpoint    string
	S3AccessKeyID string
	S3SecretKey   string
	S3ForcePath   bool
	PublicBaseURL string
	PresignTTL    time.Duration
}

type ObjectRef struct {
	Driver Driver
	Bucket string
	Key    string
	ETag   string
}

type RecordingStorage interface {
	// Driver 返回底层存储驱动类型，便于调用方在不破坏封装的前提下决定是否进行 S3 上传等行为。
	// Driver returns the underlying storage driver so callers can decide behavior (e.g. S3 upload) without breaking encapsulation.
	Driver() Driver
	SaveFile(ctx context.Context, srcPath, objectKey, contentType string) (*ObjectRef, error)
	SignedDownloadURL(ctx context.Context, objectRef ObjectRef, ttl time.Duration) (string, error)
	Open(ctx context.Context, objectRef ObjectRef) (io.ReadCloser, error)
	OpenLocal(objectRef ObjectRef) (string, bool)
	Delete(ctx context.Context, objectRef ObjectRef) error
}

func NewRecordingStorage(cfg Config) (RecordingStorage, error) {
	switch cfg.Driver {
	case "", DriverLocal:
		root := strings.TrimSpace(cfg.LocalRoot)
		if root == "" {
			root = "./recordings"
		}
		return &localRecordingStorage{root: root}, nil
	case DriverS3:
		return newS3RecordingStorage(cfg)
	default:
		return nil, fmt.Errorf("unsupported recording storage driver: %s", cfg.Driver)
	}
}

type localRecordingStorage struct {
	root string
}

func (s *localRecordingStorage) Driver() Driver { return DriverLocal }

func (s *localRecordingStorage) SaveFile(_ context.Context, srcPath, objectKey, _ string) (*ObjectRef, error) {
	srcPath = strings.TrimSpace(srcPath)
	objectKey = strings.TrimSpace(objectKey)
	if srcPath == "" || objectKey == "" {
		return nil, errors.New("source path and object key are required")
	}
	targetPath, err := s.resolvePath(objectKey)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return nil, err
	}
	if srcPath != targetPath {
		if err := copyFile(srcPath, targetPath); err != nil {
			return nil, err
		}
	}
	return &ObjectRef{
		Driver: DriverLocal,
		Key:    targetPath,
	}, nil
}

func (s *localRecordingStorage) SignedDownloadURL(_ context.Context, objectRef ObjectRef, _ time.Duration) (string, error) {
	path, ok := s.OpenLocal(objectRef)
	if !ok {
		return "", os.ErrNotExist
	}
	return path, nil
}

func (s *localRecordingStorage) OpenLocal(objectRef ObjectRef) (string, bool) {
	if objectRef.Driver != DriverLocal {
		return "", false
	}
	path, err := s.resolveExistingPath(objectRef.Key)
	if err != nil {
		return "", false
	}
	return path, true
}

func (s *localRecordingStorage) Open(_ context.Context, objectRef ObjectRef) (io.ReadCloser, error) {
	localPath, ok := s.OpenLocal(objectRef)
	if !ok {
		return nil, os.ErrNotExist
	}
	return os.Open(localPath)
}

func (s *localRecordingStorage) Delete(_ context.Context, objectRef ObjectRef) error {
	path, ok := s.OpenLocal(objectRef)
	if !ok {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *localRecordingStorage) resolvePath(objectKey string) (string, error) {
	cleanKey := filepath.Clean(filepath.FromSlash(strings.TrimSpace(objectKey)))
	if cleanKey == "." || cleanKey == ".." || filepath.IsAbs(cleanKey) || strings.HasPrefix(cleanKey, fmt.Sprintf("..%c", filepath.Separator)) {
		return "", errors.New("recording object key escapes storage root")
	}
	root, err := filepath.Abs(s.root)
	if err != nil {
		return "", err
	}
	target := filepath.Join(root, cleanKey)
	if !isPathInsideRoot(root, target) {
		return "", errors.New("recording object key escapes storage root")
	}
	return target, nil
}

func (s *localRecordingStorage) resolveExistingPath(value string) (string, error) {
	root, err := filepath.Abs(s.root)
	if err != nil {
		return "", err
	}
	target := strings.TrimSpace(value)
	if target == "" {
		return "", errors.New("recording object path is empty")
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if !isPathInsideRoot(root, target) {
		return "", errors.New("recording object path escapes storage root")
	}
	return target, nil
}

func isPathInsideRoot(root, target string) bool {
	return target == root || strings.HasPrefix(target, fmt.Sprintf("%s%c", root, filepath.Separator))
}

type s3RecordingStorage struct {
	client     *s3.Client
	presign    *s3.PresignClient
	bucket     string
	publicBase string
	defaultTTL time.Duration
}

func (s *s3RecordingStorage) Driver() Driver { return DriverS3 }

func newS3RecordingStorage(cfg Config) (RecordingStorage, error) {
	if strings.TrimSpace(cfg.S3Bucket) == "" {
		return nil, errors.New("RECORDING_S3_BUCKET is required")
	}
	region := strings.TrimSpace(cfg.S3Region)
	if region == "" {
		region = "us-east-1"
	}
	loadOptions := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(region),
	}
	if strings.TrimSpace(cfg.S3AccessKeyID) != "" || strings.TrimSpace(cfg.S3SecretKey) != "" {
		loadOptions = append(loadOptions, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.S3AccessKeyID, cfg.S3SecretKey, ""),
		))
	}
	if strings.TrimSpace(cfg.S3Endpoint) != "" {
		loadOptions = append(loadOptions, awsconfig.WithBaseEndpoint(cfg.S3Endpoint))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), loadOptions...)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.UsePathStyle = cfg.S3ForcePath
	})
	ttl := cfg.PresignTTL
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return &s3RecordingStorage{
		client:     client,
		presign:    s3.NewPresignClient(client),
		bucket:     cfg.S3Bucket,
		publicBase: strings.TrimSpace(cfg.PublicBaseURL),
		defaultTTL: ttl,
	}, nil
}

func (s *s3RecordingStorage) SaveFile(ctx context.Context, srcPath, objectKey, contentType string) (*ObjectRef, error) {
	normalizedKey, err := normalizeS3ObjectKey(objectKey)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(strings.TrimSpace(srcPath))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	uploader := manager.NewUploader(s.client)
	input := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(normalizedKey),
		Body:        file,
		ContentType: aws.String(strings.TrimSpace(contentType)),
	}
	result, err := uploader.Upload(ctx, input)
	if err != nil {
		return nil, err
	}
	etag := strings.Trim(aws.ToString(result.ETag), "\"")
	return &ObjectRef{
		Driver: DriverS3,
		Bucket: s.bucket,
		Key:    normalizedKey,
		ETag:   etag,
	}, nil
}

func (s *s3RecordingStorage) SignedDownloadURL(ctx context.Context, objectRef ObjectRef, ttl time.Duration) (string, error) {
	bucket, key, err := s.resolveObjectRef(objectRef)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(s.publicBase) != "" {
		base, err := url.JoinPath(s.publicBase, key)
		if err == nil {
			return base, nil
		}
	}
	if ttl <= 0 {
		ttl = s.defaultTTL
	}
	request, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, func(options *s3.PresignOptions) {
		options.Expires = ttl
	})
	if err != nil {
		return "", err
	}
	return request.URL, nil
}

func (s *s3RecordingStorage) OpenLocal(_ ObjectRef) (string, bool) {
	return "", false
}

func (s *s3RecordingStorage) Open(ctx context.Context, objectRef ObjectRef) (io.ReadCloser, error) {
	bucket, key, err := s.resolveObjectRef(objectRef)
	if err != nil {
		return nil, err
	}
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	return result.Body, nil
}

func (s *s3RecordingStorage) Delete(ctx context.Context, objectRef ObjectRef) error {
	bucket, key, err := s.resolveObjectRef(objectRef)
	if err != nil {
		return err
	}
	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	return err
}

func (s *s3RecordingStorage) resolveObjectRef(objectRef ObjectRef) (string, string, error) {
	if objectRef.Driver != "" && objectRef.Driver != DriverS3 {
		return "", "", errors.New("recording object driver is not s3")
	}
	bucket := strings.TrimSpace(objectRef.Bucket)
	if bucket == "" {
		bucket = s.bucket
	}
	key, err := normalizeS3ObjectKey(objectRef.Key)
	if err != nil {
		return "", "", err
	}
	return bucket, key, nil
}

func normalizeS3ObjectKey(value string) (string, error) {
	key := strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if key == "" || strings.HasPrefix(key, "/") {
		return "", errors.New("recording object key is invalid")
	}

	parts := strings.Split(key, "/")
	cleanParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		if part == "." || part == ".." {
			return "", errors.New("recording object key is invalid")
		}
		cleanParts = append(cleanParts, part)
	}
	if len(cleanParts) == 0 {
		return "", errors.New("recording object key is invalid")
	}
	return path.Join(cleanParts...), nil
}

func copyFile(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return dst.Close()
}
