package services

import (
	"bytes"
	"context"
	"fmt"
	"log"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// StorageService wraps MinIO (S3-compatible) object storage.
// It is optional — when not configured all Upload calls return "" (no key) and
// callers must handle that gracefully.
type StorageService struct {
	client     *minio.Client
	bucket     string
	publicBase string // optional: public URL prefix for direct downloads
}

// NewStorageService creates a MinIO client. Returns nil if endpoint is empty
// so callers can skip storage without crashing.
func NewStorageService(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*StorageService, error) {
	if endpoint == "" {
		return nil, nil //nolint:nilnil
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}
	return &StorageService{client: client, bucket: bucket}, nil
}

// EnsureBucket creates the bucket if it does not exist yet.
func (s *StorageService) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("bucket exists check: %w", err)
	}
	if !exists {
		if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("make bucket: %w", err)
		}
		log.Printf("[storage] created bucket %q", s.bucket)
	}
	return nil
}

// Upload stores data at the given object key and returns the key on success.
func (s *StorageService) Upload(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	if s == nil || s.client == nil {
		return "", nil
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return "", fmt.Errorf("minio put %q: %w", key, err)
	}
	return key, nil
}

// Delete removes an object. Errors are logged but not fatal.
func (s *StorageService) Delete(ctx context.Context, key string) {
	if s == nil || s.client == nil || key == "" {
		return
	}
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		log.Printf("[storage] delete %q: %v", key, err)
	}
}

// ObjectKey builds a canonical object key for a document.
// Format: {workspaceID}/{docID}/{filename}
func ObjectKey(workspaceID, docID, filename string) string {
	return fmt.Sprintf("%s/%s/%s", workspaceID, docID, filename)
}
