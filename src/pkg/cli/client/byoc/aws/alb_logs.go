package aws

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws/cw"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func (b *ByocAws) fetchAndStreamAlbLogs(ctx context.Context, projectName string, since, end time.Time) (<-chan cw.LogEvent, error) {
	cfg, err := b.driver.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	s3Client := s3.NewFromConfig(cfg)
	bucketsOutput, err := s3Client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, err
	}

	bucketPrefix := fmt.Sprintf("%s-%s-%s-alb-logs", b.Prefix, projectName, b.PulumiStack)
	term.Debug("Query ALB logs", bucketPrefix)
	if len(bucketPrefix) > 31 {
		// HACK: AWS CD truncates the ALB name to 31 characters (because of the long Terraform suffix)
		bucketPrefix = bucketPrefix[:31]
	}
	bucketPrefix = strings.ToLower(bucketPrefix)

	// First, find bucket with the given prefix for the project/stack
	var bucketName string
	for _, bucket := range bucketsOutput.Buckets {
		if strings.HasPrefix(*bucket.Name, bucketPrefix) {
			// TODO: inspect the bucket tags to ensure it belongs to the right org/project/stack
			bucketName = *bucket.Name
			break
		}
	}

	if bucketName == "" {
		return nil, fmt.Errorf("no bucket found with prefix %q", bucketPrefix)
	}

	// Start goroutine to fetch and stream ALB logs from the bucket
	logEventsChan := make(chan cw.LogEvent)
	go func() {
		defer close(logEventsChan)
		_ = b.fetchAndStreamAlbLogsFromBucket(ctx, bucketName, since, end, s3Client, logEventsChan)
	}()
	return logEventsChan, nil
}

func (b *ByocAws) fetchAndStreamAlbLogsFromBucket(ctx context.Context, bucketName string, since, end time.Time, s3Client *s3.Client, logEventsChan chan<- cw.LogEvent) error {
	if end.IsZero() {
		end = time.Now()
	}
	for since.Before(end) {
		year, month, day := since.UTC().Date()
		objectPrefix := fmt.Sprintf("AWSLogs/%s/elasticloadbalancing/%s/%04d/%02d/%02d/", b.driver.AccountID, b.driver.Region, year, month, day)

		listInput := s3.ListObjectsV2Input{
			Bucket: &bucketName,
			Prefix: &objectPrefix,
		}
		for {
			list, err := s3Client.ListObjectsV2(ctx, &listInput)
			if err != nil {
				return err
			}
			for _, obj := range list.Contents {
				if obj.LastModified.Before(since) {
					// Skip objects with events older than the since-time
					continue
				}
				// Check end-time, but consider that each object has ~5 minutes of logs
				oldestEntry := obj.LastModified.Add(-5 * time.Minute)
				if !end.IsZero() && oldestEntry.After(end) {
					return nil
				}
				if err := streamAlbLogsFromS3(ctx, bucketName, *obj.Key, s3Client, logEventsChan); err != nil {
					return err // or skip?
				}
			}
			if list.NextContinuationToken == nil {
				break
			}
			listInput.ContinuationToken = list.NextContinuationToken
		}
		// Done? TODO: keep looping next day's logs
		since = time.Date(year, month, day+1, 0, 0, 0, 0, time.UTC)
	}
	return io.EOF
}

func streamAlbLogsFromS3(ctx context.Context, bucketName string, objKey string, s3Client *s3.Client, logEventsChan chan<- cw.LogEvent) error {
	content, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucketName,
		Key:    &objKey,
	})
	if err != nil {
		return err
	}
	defer content.Body.Close()
	gzipReader, err := gzip.NewReader(content.Body)
	if err != nil {
		return err
	}
	lastModified := content.LastModified.UnixMilli()
	defer gzipReader.Close()
	lineScanner := bufio.NewScanner(gzipReader)
	for lineScanner.Scan() {
		line := lineScanner.Text()
		var timestamp int64
		if parts := strings.SplitN(line, " ", 3); len(parts) != 3 {
			continue // malformed line
		} else if ts, err := time.Parse(time.RFC3339Nano, parts[1]); err != nil {
			continue // malformed timestamp
		} else {
			timestamp = ts.UnixMilli()
		}
		select {
		case logEventsChan <- cw.LogEvent{
			IngestionTime: &lastModified,
			Message:       &line, // TODO: parse ALB log format
			Timestamp:     &timestamp,
		}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
