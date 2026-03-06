package archive

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeConfigDefaults(t *testing.T) {
	t.Setenv("FLOW_TELEMETRY_ARCHIVE_MODE", "")
	t.Setenv("FLOW_TELEMETRY_ARCHIVE_LOCAL_DIR", "")

	cfg := RuntimeConfigFromEnv()
	if cfg.NormalizedMode() != ModeDisabled {
		t.Fatalf("expected disabled mode, got %q", cfg.NormalizedMode())
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected default config to validate: %v", err)
	}
}

func TestRuntimeConfigLocalMode(t *testing.T) {
	t.Setenv("FLOW_TELEMETRY_ARCHIVE_MODE", "LOCAL")
	t.Setenv("FLOW_TELEMETRY_ARCHIVE_LOCAL_DIR", "./tmp/archive")

	cfg := RuntimeConfigFromEnv()
	if cfg.NormalizedMode() != ModeLocal {
		t.Fatalf("expected local mode, got %q", cfg.NormalizedMode())
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected local config to validate: %v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(cfg.ResolvedLocalDir()), "tmp/archive") {
		t.Fatalf("unexpected resolved local dir: %s", cfg.ResolvedLocalDir())
	}
}

func TestRuntimeConfigRejectsUnknownMode(t *testing.T) {
	t.Setenv("FLOW_TELEMETRY_ARCHIVE_MODE", "unknown")
	cfg := RuntimeConfigFromEnv()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for unsupported mode")
	}
}
