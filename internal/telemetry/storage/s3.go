package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

type S3Client interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

type S3Store struct {
	bucket string
	client S3Client
	now    func() time.Time
}

func NewS3Store(bucket string, client S3Client) *S3Store {
	return &S3Store{
		bucket: bucket,
		client: client,
		now:    time.Now,
	}
}

func (s *S3Store) Put(ctx context.Context, key string, data []byte, metadata map[string]string) error {
	safeKey, err := sanitizeKey(key)
	if err != nil {
		return err
	}

	meta := copyMetadata(metadata)
	meta["created_at"] = s.now().UTC().Format(time.RFC3339Nano)

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:   awsString(s.bucket),
		Key:      awsString(safeKey),
		Body:     bytes.NewReader(data),
		Metadata: meta,
	})
	return err
}

func (s *S3Store) Get(ctx context.Context, key string) (Object, error) {
	safeKey, err := sanitizeKey(key)
	if err != nil {
		return Object{}, err
	}

	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: awsString(s.bucket),
		Key:    awsString(safeKey),
	})
	if err != nil {
		if isNotFound(err) {
			return Object{}, ErrNotFound
		}
		return Object{}, err
	}
	defer output.Body.Close()

	data, err := io.ReadAll(output.Body)
	if err != nil {
		return Object{}, err
	}

	metadata := copyMetadata(output.Metadata)
	createdAt := parseCreatedAt(metadata["created_at"], output.LastModified)

	return Object{
		Key:       safeKey,
		Data:      data,
		Metadata:  metadata,
		CreatedAt: createdAt,
	}, nil
}

func (s *S3Store) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	infos := make([]ObjectInfo, 0)
	var token *string

	for {
		output, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            awsString(s.bucket),
			Prefix:            awsString(prefix),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, err
		}

		for _, entry := range output.Contents {
			createdAt := time.Time{}
			if entry.LastModified != nil {
				createdAt = entry.LastModified.UTC()
			}
			infos = append(infos, ObjectInfo{
				Key:       derefString(entry.Key),
				CreatedAt: createdAt,
			})
		}

		if output.NextContinuationToken == nil || *output.NextContinuationToken == "" {
			break
		}
		token = output.NextContinuationToken
	}

	slices.SortFunc(infos, func(a, b ObjectInfo) int {
		if a.Key < b.Key {
			return -1
		}
		if a.Key > b.Key {
			return 1
		}
		return 0
	})

	return infos, nil
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	safeKey, err := sanitizeKey(key)
	if err != nil {
		return err
	}

	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: awsString(s.bucket),
		Key:    awsString(safeKey),
	})
	return err
}

func (s *S3Store) ApplyRetention(ctx context.Context, policy RetentionPolicy) ([]ObjectInfo, error) {
	if policy.MaxAge <= 0 {
		return []ObjectInfo{}, nil
	}

	infos, err := s.List(ctx, policy.Prefix)
	if err != nil {
		return nil, err
	}

	cutoff := s.now().UTC().Add(-policy.MaxAge)
	candidates := make([]ObjectInfo, 0)

	for _, info := range infos {
		if info.CreatedAt.IsZero() || !info.CreatedAt.Before(cutoff) {
			continue
		}

		candidates = append(candidates, info)
		if policy.OnDeleteCandidate != nil {
			policy.OnDeleteCandidate(info)
		}

		if policy.DryRun {
			continue
		}
		if err := s.Delete(ctx, info.Key); err != nil {
			return nil, err
		}
	}

	return candidates, nil
}

func isNotFound(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	code := strings.ToLower(apiErr.ErrorCode())
	return code == "nosuchkey" || code == "notfound"
}

func parseCreatedAt(value string, fallback *time.Time) time.Time {
	if value != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return parsed.UTC()
		}
	}
	if fallback != nil {
		return fallback.UTC()
	}
	return time.Time{}
}

func awsString(v string) *string {
	return &v
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
