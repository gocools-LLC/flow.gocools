package storage

import (
	"context"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	internalaws "github.com/gocools-LLC/flow.gocools/internal/aws"
)

type S3ClientConfig struct {
	Region      string
	RoleARN     string
	SessionName string
	ExternalID  string
}

func NewAWSS3Client(ctx context.Context, cfg S3ClientConfig, optFns ...func(*awsconfig.LoadOptions) error) (*s3.Client, error) {
	awsCfg, err := internalaws.LoadConfig(ctx, internalaws.SessionConfig{
		Region:      cfg.Region,
		RoleARN:     cfg.RoleARN,
		SessionName: cfg.SessionName,
		ExternalID:  cfg.ExternalID,
	}, optFns...)
	if err != nil {
		return nil, err
	}

	return s3.NewFromConfig(awsCfg), nil
}
