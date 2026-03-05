package storage

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

func TestS3StorePutGetListDelete(t *testing.T) {
	fake := newFakeS3Client()
	store := NewS3Store("test-bucket", fake)
	store.now = func() time.Time { return time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC) }

	if err := store.Put(context.Background(), "telemetry/log-1.json", []byte("hello"), map[string]string{"type": "log"}); err != nil {
		t.Fatalf("put failed: %v", err)
	}

	obj, err := store.Get(context.Background(), "telemetry/log-1.json")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if string(obj.Data) != "hello" {
		t.Fatalf("unexpected object data: %q", string(obj.Data))
	}
	if obj.Metadata["type"] != "log" {
		t.Fatalf("expected metadata type log, got %q", obj.Metadata["type"])
	}

	infos, err := store.List(context.Background(), "telemetry/")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(infos) != 1 || infos[0].Key != "telemetry/log-1.json" {
		t.Fatalf("unexpected list result: %+v", infos)
	}

	if err := store.Delete(context.Background(), "telemetry/log-1.json"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if _, err := store.Get(context.Background(), "telemetry/log-1.json"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestS3StoreApplyRetention(t *testing.T) {
	fake := newFakeS3Client()
	oldTime := time.Date(2026, 3, 5, 8, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC)

	fake.objects["telemetry/old.log"] = fakeS3Object{
		data:         []byte("old"),
		metadata:     map[string]string{"created_at": oldTime.Format(time.RFC3339Nano)},
		lastModified: oldTime,
	}
	fake.objects["telemetry/new.log"] = fakeS3Object{
		data:         []byte("new"),
		metadata:     map[string]string{"created_at": newTime.Format(time.RFC3339Nano)},
		lastModified: newTime,
	}

	store := NewS3Store("test-bucket", fake)
	store.now = func() time.Time { return time.Date(2026, 3, 5, 10, 30, 0, 0, time.UTC) }

	candidates, err := store.ApplyRetention(context.Background(), RetentionPolicy{
		Prefix: "telemetry/",
		MaxAge: 90 * time.Minute,
	})
	if err != nil {
		t.Fatalf("apply retention failed: %v", err)
	}

	if len(candidates) != 1 || candidates[0].Key != "telemetry/old.log" {
		t.Fatalf("unexpected retention candidates: %+v", candidates)
	}

	if _, exists := fake.objects["telemetry/old.log"]; exists {
		t.Fatal("expected old object to be deleted")
	}
	if _, exists := fake.objects["telemetry/new.log"]; !exists {
		t.Fatal("expected new object to remain")
	}
}

type fakeS3Client struct {
	objects map[string]fakeS3Object
}

type fakeS3Object struct {
	data         []byte
	metadata     map[string]string
	lastModified time.Time
}

func newFakeS3Client() *fakeS3Client {
	return &fakeS3Client{
		objects: map[string]fakeS3Object{},
	}
}

func (f *fakeS3Client) PutObject(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	body, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}

	key := derefString(input.Key)
	f.objects[key] = fakeS3Object{
		data:         body,
		metadata:     copyMetadata(input.Metadata),
		lastModified: parseCreatedAt(input.Metadata["created_at"], nil),
	}
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeS3Client) GetObject(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	key := derefString(input.Key)
	object, exists := f.objects[key]
	if !exists {
		return nil, &smithy.GenericAPIError{
			Code:    "NoSuchKey",
			Message: "key not found",
		}
	}

	lastModified := object.lastModified
	return &s3.GetObjectOutput{
		Body:         io.NopCloser(bytes.NewReader(object.data)),
		Metadata:     copyMetadata(object.metadata),
		LastModified: &lastModified,
	}, nil
}

func (f *fakeS3Client) DeleteObject(_ context.Context, input *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	delete(f.objects, derefString(input.Key))
	return &s3.DeleteObjectOutput{}, nil
}

func (f *fakeS3Client) ListObjectsV2(_ context.Context, input *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	prefix := derefString(input.Prefix)
	contents := make([]types.Object, 0)
	for key, object := range f.objects {
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			continue
		}
		keyCopy := key
		lastModified := object.lastModified
		contents = append(contents, types.Object{
			Key:          &keyCopy,
			LastModified: &lastModified,
		})
	}

	return &s3.ListObjectsV2Output{
		Contents: contents,
	}, nil
}
