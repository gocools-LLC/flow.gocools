package cloudwatch

import (
	"context"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	sdkcloudwatch "github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	internalaws "github.com/gocools-LLC/flow.gocools/internal/aws"
)

type ClientConfig struct {
	Region      string
	RoleARN     string
	SessionName string
	ExternalID  string
}

func NewAWSClient(ctx context.Context, cfg ClientConfig, optFns ...func(*awsconfig.LoadOptions) error) (*sdkcloudwatch.Client, error) {
	awsCfg, err := internalaws.LoadConfig(ctx, internalaws.SessionConfig{
		Region:      cfg.Region,
		RoleARN:     cfg.RoleARN,
		SessionName: cfg.SessionName,
		ExternalID:  cfg.ExternalID,
	}, optFns...)
	if err != nil {
		return nil, err
	}

	return sdkcloudwatch.NewFromConfig(awsCfg), nil
}
