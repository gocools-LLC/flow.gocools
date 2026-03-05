package storage

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("object not found")

type Object struct {
	Key       string
	Data      []byte
	Metadata  map[string]string
	CreatedAt time.Time
}

type ObjectInfo struct {
	Key       string
	CreatedAt time.Time
}

type RetentionPolicy struct {
	Prefix            string
	MaxAge            time.Duration
	DryRun            bool
	OnDeleteCandidate func(info ObjectInfo)
}

type Store interface {
	Put(ctx context.Context, key string, data []byte, metadata map[string]string) error
	Get(ctx context.Context, key string) (Object, error)
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)
	Delete(ctx context.Context, key string) error
	ApplyRetention(ctx context.Context, policy RetentionPolicy) ([]ObjectInfo, error)
}
