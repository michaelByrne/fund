package aws

import (
	"boardfund/service/finance"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
	"io"
	"log/slog"
	"regexp"
	"time"
)

type AWSS3 struct {
	s3Client *s3.Client

	logger *slog.Logger

	bucket string
}

func NewAWSS3(s3Client *s3.Client, logger *slog.Logger, bucket string) *AWSS3 {
	return &AWSS3{
		s3Client: s3Client,
		logger:   logger,
		bucket:   bucket,
	}
}

func (s AWSS3) CreateFundBucket(ctx context.Context, prefix string, fundID uuid.UUID) error {
	input := s3.CreateBucketInput{
		Bucket: toPointer(prefix + "." + fundID.String()),
		CreateBucketConfiguration: &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraintUsWest2,
		},
	}

	_, err := s.s3Client.CreateBucket(ctx, &input)
	if err != nil {
		s.logger.Error("failed to create bucket", slog.String("error", err.Error()))

		return err
	}

	return nil
}

func (s AWSS3) Upload(ctx context.Context, body io.Reader, name, contentType string) error {
	// Read all content into memory so we can use it twice
	content, err := io.ReadAll(body)
	if err != nil {
		s.logger.Error("failed to read body", slog.String("error", err.Error()))
		return err
	}

	// Calculate MD5
	hasher := md5.New()
	hasher.Write(content)
	md5Hash := hasher.Sum(nil)
	bodyHash := base64.StdEncoding.EncodeToString(md5Hash)

	reportInfo, err := parseReportKey(name)
	if err != nil {
		s.logger.Error("failed to parse report key", slog.String("error", err.Error()))
		return err
	}

	bucket := reportInfo.Type + "." + reportInfo.FundID.String()

	input := s3.PutObjectInput{
		Bucket:      &bucket,
		Key:         &name,
		ContentType: &contentType,
		Body:        bytes.NewReader(content), // Create new reader from the content
		ContentMD5:  &bodyHash,
	}

	_, err = s.s3Client.PutObject(ctx, &input)
	if err != nil {
		s.logger.Error("failed to upload to s3", slog.String("error", err.Error()))
		return err
	}

	return nil
}

func (s AWSS3) ListAvailableReports(ctx context.Context, prefix string, fundID uuid.UUID) ([]finance.ReportInfo, error) {
	input := s3.ListObjectsV2Input{
		Bucket: toPointer(prefix + "." + fundID.String()),
	}

	output, err := s.s3Client.ListObjectsV2(ctx, &input)
	if err != nil {
		s.logger.Error("failed to list objects in s3", slog.String("error", err.Error()))

		return nil, err
	}

	var info []finance.ReportInfo
	for _, object := range output.Contents {
		key := *object.Key
		reportInfo, errParse := parseReportKey(key)
		if errParse != nil {
			s.logger.Error("failed to parse key", slog.String("error", errParse.Error()))

			continue
		}

		info = append(info, *reportInfo)
	}

	return info, nil
}

func parseReportKey(key string) (*finance.ReportInfo, error) {
	re := regexp.MustCompile(`^fund_(.*?)_date_(\d{2}-\d{2}-\d{4})_(\w+)_report\.csv$`)

	// Apply the regular expression to the fileName
	matches := re.FindStringSubmatch(key)

	if len(matches) != 4 {
		return nil, fmt.Errorf("failed to match key: %s", key)
	}

	fundID := matches[1]
	dateStr := matches[2]
	reportType := matches[3]

	date, err := time.Parse("01-02-2006", dateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse date: %s", dateStr)
	}

	fundUUID, err := uuid.Parse(fundID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse fundID: %s", fundID)
	}

	return &finance.ReportInfo{
		FundID: fundUUID,
		Date:   date,
		Type:   reportType,
	}, nil
}

func toB64MD5(body io.Reader) (string, error) {
	hasher := md5.New()

	if _, err := io.Copy(hasher, body); err != nil {
		return "", err
	}

	md5Hash := hasher.Sum(nil)

	return base64.StdEncoding.EncodeToString(md5Hash), nil
}
