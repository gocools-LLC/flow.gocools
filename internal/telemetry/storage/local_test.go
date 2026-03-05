package storage

import (
	"context"
	"testing"
	"time"
)

func TestLocalStorePutGetList(t *testing.T) {
	store := NewLocalStore(t.TempDir())
	store.now = func() time.Time { return time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC) }

	if err := store.Put(context.Background(), "ecs/metrics-1.json", []byte(`{"ok":true}`), map[string]string{"type": "metric"}); err != nil {
		t.Fatalf("put failed: %v", err)
	}

	obj, err := store.Get(context.Background(), "ecs/metrics-1.json")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}

	if string(obj.Data) != `{"ok":true}` {
		t.Fatalf("unexpected object data: %q", string(obj.Data))
	}
	if obj.Metadata["type"] != "metric" {
		t.Fatalf("expected metadata type metric, got %q", obj.Metadata["type"])
	}

	infos, err := store.List(context.Background(), "ecs/")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(infos) != 1 || infos[0].Key != "ecs/metrics-1.json" {
		t.Fatalf("unexpected list result: %+v", infos)
	}
}

func TestLocalStoreApplyRetention(t *testing.T) {
	store := NewLocalStore(t.TempDir())
	store.now = func() time.Time { return time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC) }

	if err := store.Put(context.Background(), "logs/old.log", []byte("old"), nil); err != nil {
		t.Fatalf("put old failed: %v", err)
	}

	store.now = func() time.Time { return time.Date(2026, 3, 5, 10, 10, 0, 0, time.UTC) }
	if err := store.Put(context.Background(), "logs/new.log", []byte("new"), nil); err != nil {
		t.Fatalf("put new failed: %v", err)
	}

	store.now = func() time.Time { return time.Date(2026, 3, 5, 10, 20, 0, 0, time.UTC) }

	dryRunCandidates, err := store.ApplyRetention(context.Background(), RetentionPolicy{
		Prefix: "logs/",
		MaxAge: 15 * time.Minute,
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("retention dry-run failed: %v", err)
	}
	if len(dryRunCandidates) != 1 || dryRunCandidates[0].Key != "logs/old.log" {
		t.Fatalf("unexpected dry-run candidates: %+v", dryRunCandidates)
	}

	if _, err := store.Get(context.Background(), "logs/old.log"); err != nil {
		t.Fatalf("old object should remain after dry-run: %v", err)
	}

	deletedCandidates, err := store.ApplyRetention(context.Background(), RetentionPolicy{
		Prefix: "logs/",
		MaxAge: 15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("retention delete failed: %v", err)
	}
	if len(deletedCandidates) != 1 || deletedCandidates[0].Key != "logs/old.log" {
		t.Fatalf("unexpected deleted candidates: %+v", deletedCandidates)
	}

	if _, err := store.Get(context.Background(), "logs/old.log"); err == nil {
		t.Fatal("expected old object to be deleted")
	}
}
