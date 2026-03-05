package aws

import (
	"context"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type SessionConfig struct {
	Region      string
	RoleARN     string
	SessionName string
	ExternalID  string
}

func LoadConfig(ctx context.Context, cfg SessionConfig, optFns ...func(*config.LoadOptions) error) (sdkaws.Config, error) {
	loadOptions := make([]func(*config.LoadOptions) error, 0, len(optFns)+1)
	if cfg.Region != "" {
		loadOptions = append(loadOptions, config.WithRegion(cfg.Region))
	}
	loadOptions = append(loadOptions, optFns...)

	awsCfg, err := config.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return sdkaws.Config{}, err
	}

	if cfg.RoleARN == "" {
		return awsCfg, nil
	}

	sessionName := cfg.SessionName
	if sessionName == "" {
		sessionName = "flow-session"
	}

	assumeRoleProvider := stscreds.NewAssumeRoleProvider(sts.NewFromConfig(awsCfg), cfg.RoleARN, func(options *stscreds.AssumeRoleOptions) {
		options.RoleSessionName = sessionName
		if cfg.ExternalID != "" {
			options.ExternalID = &cfg.ExternalID
		}
	})

	awsCfg.Credentials = sdkaws.NewCredentialsCache(assumeRoleProvider)
	return awsCfg, nil
}
