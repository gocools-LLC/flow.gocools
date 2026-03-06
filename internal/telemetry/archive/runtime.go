package archive

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	ModeDisabled = "disabled"
	ModeLocal    = "local"
)

type RuntimeConfig struct {
	Mode     string
	LocalDir string
}

func RuntimeConfigFromEnv() RuntimeConfig {
	return RuntimeConfig{
		Mode:     envOrDefault("FLOW_TELEMETRY_ARCHIVE_MODE", ModeDisabled),
		LocalDir: envOrDefault("FLOW_TELEMETRY_ARCHIVE_LOCAL_DIR", "./dist/telemetry"),
	}
}

func (c RuntimeConfig) NormalizedMode() string {
	mode := strings.ToLower(strings.TrimSpace(c.Mode))
	if mode == "" {
		return ModeDisabled
	}
	return mode
}

func (c RuntimeConfig) Validate() error {
	switch c.NormalizedMode() {
	case ModeDisabled:
		return nil
	case ModeLocal:
		if strings.TrimSpace(c.LocalDir) == "" {
			return fmt.Errorf("FLOW_TELEMETRY_ARCHIVE_LOCAL_DIR is required for local archive mode")
		}
		return nil
	default:
		return fmt.Errorf("unsupported FLOW_TELEMETRY_ARCHIVE_MODE: %q", c.Mode)
	}
}

func (c RuntimeConfig) ResolvedLocalDir() string {
	dir := strings.TrimSpace(c.LocalDir)
	if dir == "" {
		dir = "./dist/telemetry"
	}
	if filepath.IsAbs(dir) {
		return dir
	}

	wd, err := os.Getwd()
	if err != nil {
		return dir
	}
	return filepath.Clean(filepath.Join(wd, dir))
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
