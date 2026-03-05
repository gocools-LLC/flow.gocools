package aws

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/config"
)

type RuntimeConfig struct {
	Session         SessionConfig
	ValidateOnStart bool
}

func RuntimeConfigFromEnv() RuntimeConfig {
	return RuntimeConfig{
		Session: SessionConfig{
			Region:      os.Getenv("FLOW_AWS_REGION"),
			RoleARN:     os.Getenv("FLOW_AWS_ROLE_ARN"),
			SessionName: envOrDefault("FLOW_AWS_SESSION_NAME", "flow-session"),
			ExternalID:  os.Getenv("FLOW_AWS_EXTERNAL_ID"),
		},
		ValidateOnStart: envBoolOrDefault("FLOW_AWS_VALIDATE_ON_START", false),
	}
}

func ValidateCredentials(ctx context.Context, session SessionConfig, optFns ...func(*config.LoadOptions) error) error {
	awsCfg, err := LoadConfig(ctx, session, optFns...)
	if err != nil {
		return fmt.Errorf("load aws config: %w", err)
	}

	if awsCfg.Credentials == nil {
		return fmt.Errorf("credentials provider is not configured")
	}

	if _, err := awsCfg.Credentials.Retrieve(ctx); err != nil {
		return fmt.Errorf("retrieve aws credentials: %w", err)
	}

	return nil
}

func envOrDefault(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envBoolOrDefault(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
