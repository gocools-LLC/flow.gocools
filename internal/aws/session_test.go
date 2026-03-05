package aws

import (
	"context"
	"testing"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

func TestLoadConfigWithoutAssumeRole(t *testing.T) {
	awsCfg, err := LoadConfig(context.Background(), SessionConfig{
		Region: "us-east-1",
	}, config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
		Value: sdkaws.Credentials{
			AccessKeyID:     "test-access-key",
			SecretAccessKey: "test-secret",
		},
	}))
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}

	if awsCfg.Region != "us-east-1" {
		t.Fatalf("expected region us-east-1, got %q", awsCfg.Region)
	}

	creds, err := awsCfg.Credentials.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("retrieve credentials failed: %v", err)
	}
	if creds.AccessKeyID != "test-access-key" {
		t.Fatalf("expected access key test-access-key, got %q", creds.AccessKeyID)
	}
}

func TestLoadConfigWithAssumeRoleSetsCredentialsProvider(t *testing.T) {
	awsCfg, err := LoadConfig(context.Background(), SessionConfig{
		Region:      "us-west-2",
		RoleARN:     "arn:aws:iam::123456789012:role/test-role",
		SessionName: "flow-unit-test",
	}, config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
		Value: sdkaws.Credentials{
			AccessKeyID:     "test-access-key",
			SecretAccessKey: "test-secret",
		},
	}))
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}

	if awsCfg.Region != "us-west-2" {
		t.Fatalf("expected region us-west-2, got %q", awsCfg.Region)
	}

	if awsCfg.Credentials == nil {
		t.Fatal("expected credentials provider to be configured")
	}
}
