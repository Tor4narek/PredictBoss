package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awsS3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/predictboss/predictor/esb/internal/domain"
)

const MaxProfileBytes = 2 << 20

type Config struct {
	Bucket    string
	Region    string
	Endpoint  string
	AccessKey string
	SecretKey string
	PathStyle bool
}

type Store struct {
	bucket string
	client *awsS3.Client
}

func NewStore(ctx context.Context, cfg Config) (*Store, error) {
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, errors.New("S3 bucket is required")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if (cfg.AccessKey == "") != (cfg.SecretKey == "") {
		return nil, errors.New("S3 access key and secret key must be set together")
	}
	options := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(cfg.Region)}
	if cfg.AccessKey != "" {
		options = append(options, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("load S3 configuration: %w", err)
	}
	client := awsS3.NewFromConfig(awsCfg, func(options *awsS3.Options) {
		options.UsePathStyle = cfg.PathStyle
		if cfg.Endpoint != "" {
			options.BaseEndpoint = aws.String(strings.TrimRight(cfg.Endpoint, "/"))
		}
	})
	return &Store{bucket: cfg.Bucket, client: client}, nil
}

func (s *Store) Get(ctx context.Context, companyID string) (string, string, error) {
	key, err := profileKey(companyID)
	if err != nil {
		return "", "", err
	}
	result, err := s.client.GetObject(ctx, &awsS3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return "", "", fmt.Errorf("read company profile from S3: %w", err)
	}
	defer result.Body.Close()
	body, err := io.ReadAll(io.LimitReader(result.Body, MaxProfileBytes+1))
	if err != nil {
		return "", "", fmt.Errorf("read company profile body: %w", err)
	}
	if len(body) > MaxProfileBytes {
		return "", "", errors.New("company profile exceeds size limit")
	}
	if strings.TrimSpace(string(body)) == "" {
		return "", "", errors.New("company profile is empty")
	}
	return string(body), key, nil
}

func (s *Store) Save(ctx context.Context, companyID, markdown string) (string, error) {
	key, err := profileKey(companyID)
	if err != nil {
		return "", err
	}
	markdown = strings.TrimSpace(markdown)
	if markdown == "" || len(markdown) > MaxProfileBytes {
		return "", errors.New("profile markdown is empty or too large")
	}
	_, err = s.client.PutObject(ctx, &awsS3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key), Body: strings.NewReader(markdown), ContentType: aws.String("text/markdown; charset=utf-8")})
	if err != nil {
		return "", fmt.Errorf("write company profile to S3: %w", err)
	}
	return key, nil
}

func profileKey(companyID string) (string, error) {
	companyID = strings.TrimSpace(companyID)
	if len(companyID) < 1 || len(companyID) > 128 || companyID == "." || companyID == ".." {
		return "", errors.New("invalid company_id")
	}
	for _, char := range companyID {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("._-", char)) {
			return "", errors.New("invalid company_id")
		}
	}
	return "companies/" + companyID + "/profile.md", nil
}

var _ domain.ProfileRepository = (*Store)(nil)
