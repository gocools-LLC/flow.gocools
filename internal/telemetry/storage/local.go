package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type LocalStore struct {
	root string
	now  func() time.Time
}

type localMetadata struct {
	CreatedAt time.Time         `json:"created_at"`
	Metadata  map[string]string `json:"metadata"`
}

func NewLocalStore(root string) *LocalStore {
	return &LocalStore{
		root: root,
		now:  time.Now,
	}
}

func (s *LocalStore) Put(_ context.Context, key string, data []byte, metadata map[string]string) error {
	safeKey, err := sanitizeKey(key)
	if err != nil {
		return err
	}

	dataPath := filepath.Join(s.root, filepath.FromSlash(safeKey))
	if err := os.MkdirAll(filepath.Dir(dataPath), 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(dataPath, data, 0o644); err != nil {
		return err
	}

	meta := localMetadata{
		CreatedAt: s.now().UTC(),
		Metadata:  copyMetadata(metadata),
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return err
	}

	return os.WriteFile(localMetadataPath(dataPath), metaBytes, 0o644)
}

func (s *LocalStore) Get(_ context.Context, key string) (Object, error) {
	safeKey, err := sanitizeKey(key)
	if err != nil {
		return Object{}, err
	}

	dataPath := filepath.Join(s.root, filepath.FromSlash(safeKey))
	data, err := os.ReadFile(dataPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Object{}, ErrNotFound
		}
		return Object{}, err
	}

	createdAt, metadata := s.readMetadata(dataPath)
	return Object{
		Key:       safeKey,
		Data:      data,
		Metadata:  metadata,
		CreatedAt: createdAt,
	}, nil
}

func (s *LocalStore) List(_ context.Context, prefix string) ([]ObjectInfo, error) {
	infos := make([]ObjectInfo, 0)

	err := filepath.WalkDir(s.root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".meta.json") {
			return nil
		}

		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			return nil
		}

		createdAt, _ := s.readMetadata(path)
		infos = append(infos, ObjectInfo{
			Key:       key,
			CreatedAt: createdAt,
		})

		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []ObjectInfo{}, nil
		}
		return nil, err
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

func (s *LocalStore) Delete(_ context.Context, key string) error {
	safeKey, err := sanitizeKey(key)
	if err != nil {
		return err
	}

	dataPath := filepath.Join(s.root, filepath.FromSlash(safeKey))
	if err := os.Remove(dataPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(localMetadataPath(dataPath)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

func (s *LocalStore) ApplyRetention(ctx context.Context, policy RetentionPolicy) ([]ObjectInfo, error) {
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

func (s *LocalStore) readMetadata(dataPath string) (time.Time, map[string]string) {
	metaBytes, err := os.ReadFile(localMetadataPath(dataPath))
	if err != nil {
		fileInfo, statErr := os.Stat(dataPath)
		if statErr != nil {
			return time.Time{}, map[string]string{}
		}
		return fileInfo.ModTime().UTC(), map[string]string{}
	}

	var meta localMetadata
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return time.Time{}, map[string]string{}
	}
	return meta.CreatedAt.UTC(), copyMetadata(meta.Metadata)
}

func localMetadataPath(dataPath string) string {
	return dataPath + ".meta.json"
}

func sanitizeKey(key string) (string, error) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(key, "/"))
	if trimmed == "" {
		return "", errors.New("key is required")
	}

	cleaned := filepath.ToSlash(filepath.Clean(trimmed))
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || cleaned == ".." || strings.Contains(cleaned, "/../") {
		return "", errors.New("invalid key path")
	}

	return cleaned, nil
}

func copyMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return map[string]string{}
	}

	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}
