package storage

import (
	"context"
	"testing"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

func TestNewAWSS3Client(t *testing.T) {
	client, err := NewAWSS3Client(context.Background(), S3ClientConfig{
		Region: "us-east-1",
	}, awsconfig.WithCredentialsProvider(credentials.StaticCredentialsProvider{
		Value: sdkaws.Credentials{
			AccessKeyID:     "test-access-key",
			SecretAccessKey: "test-secret",
		},
	}))
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	if client == nil {
		t.Fatal("expected s3 client")
	}
}
