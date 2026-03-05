package aws

import (
	"context"
	"testing"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

func TestRuntimeConfigFromEnv(t *testing.T) {
	t.Setenv("FLOW_AWS_REGION", "us-east-1")
	t.Setenv("FLOW_AWS_ROLE_ARN", "arn:aws:iam::123456789012:role/flow")
	t.Setenv("FLOW_AWS_SESSION_NAME", "flow-test-session")
	t.Setenv("FLOW_AWS_EXTERNAL_ID", "external-id")
	t.Setenv("FLOW_AWS_VALIDATE_ON_START", "true")

	cfg := RuntimeConfigFromEnv()

	if cfg.Session.Region != "us-east-1" {
		t.Fatalf("expected region us-east-1, got %q", cfg.Session.Region)
	}
	if cfg.Session.RoleARN == "" {
		t.Fatal("expected role arn to be set")
	}
	if cfg.Session.SessionName != "flow-test-session" {
		t.Fatalf("expected custom session name, got %q", cfg.Session.SessionName)
	}
	if cfg.Session.ExternalID != "external-id" {
		t.Fatalf("expected external id, got %q", cfg.Session.ExternalID)
	}
	if !cfg.ValidateOnStart {
		t.Fatal("expected validate on start to be true")
	}
}

func TestValidateCredentialsWithStaticProvider(t *testing.T) {
	err := ValidateCredentials(context.Background(), SessionConfig{
		Region: "us-east-1",
	}, config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
		Value: sdkaws.Credentials{
			AccessKeyID:     "test-access-key",
			SecretAccessKey: "test-secret",
		},
	}))
	if err != nil {
		t.Fatalf("validate credentials failed: %v", err)
	}
}
