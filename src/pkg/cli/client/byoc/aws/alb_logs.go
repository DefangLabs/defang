package aws

import (
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"slices"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws/cw"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func (b *ByocAws) fetchAndStreamAlbLogs(ctx context.Context, projectName string, since, end time.Time) (iter.Seq2[cw.LogEvent, error], error) {
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

	return func(yield func(cw.LogEvent, error) bool) {
		for logs, err := range b.fetchAndStreamAlbLogsFromBucket(ctx, bucketName, since, end, s3Client) {
			if err != nil {
				yield(cw.LogEvent{}, err)
				return
			}
			for _, log := range logs {
				timestamp := log.Timestamp.UnixMilli()
				if !yield(cw.LogEvent{
					Message:   &log.Message,
					Timestamp: &timestamp,
				}, nil) {
					return
				}
			}
		}
	}, nil
}

type s3Lister interface {
	s3.ListObjectsV2APIClient
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

func getAlbLogObjectGroupKey(objName string) string {
	// 123456789012_elasticloadbalancing_us-test-2_app.defang-project-stack-alb.d850f5ca299e222a_20260207T0120Z_11.22.33.44_2khrazuh.log.gz
	key, _, _ := strings.Cut(objName, "Z_")
	return key
}

func (b *ByocAws) fetchAndStreamAlbLogsFromBucket(ctx context.Context, bucketName string, since, end time.Time, s3Client s3Lister) iter.Seq2[[]ALBLogEntry, error] {
	return func(yield func([]ALBLogEntry, error) bool) {
		if end.IsZero() {
			end = time.Now()
		}
		// If the end time is 00:00:01Z, we should still consider log files modified at 00:05:03Z
		// because each file has ~5 minutes of logs and writing the file will have take a few seconds.
		lastModifiedEnd := end.Add(5*time.Minute + 5*time.Second)
		for since.Before(end) {
			year, month, day := since.UTC().Date()
			objectPrefix := fmt.Sprintf("AWSLogs/%s/elasticloadbalancing/%s/%04d/%02d/%02d/", b.driver.AccountID, b.driver.Region, year, month, day)

			listInput := s3.ListObjectsV2Input{
				Bucket: &bucketName,
				Prefix: &objectPrefix,
				// StartAfter: TODO: if we know the ALB name, we can use this to quickly skip objects < since
			}
			var groupKey string
			var group []s3types.Object
			for {
				list, err := s3Client.ListObjectsV2(ctx, &listInput)
				if err != nil {
					yield(nil, err)
					return
				}
				for _, obj := range list.Contents {
					// LastModified is time of latest record. Skip objects with events older than the since-time
					if obj.LastModified.Before(since) {
						continue
					}
					// Check end-time, but consider that each object has ~5 minutes of logs
					if obj.LastModified.After(lastModifiedEnd) {
						yield(readAlbLogsGroup(ctx, bucketName, group, since, end, s3Client)) // flush last one
						return
					}
					if key := getAlbLogObjectGroupKey(*obj.Key); key == groupKey {
						// Same timespan as the previous object, so add to group for merging.
						group = append(group, obj)
					} else {
						// New timespan, so stream logs from the previous group(s) before starting a new group.
						if !yield(readAlbLogsGroup(ctx, bucketName, group, since, end, s3Client)) {
							return
						}
						group = []s3types.Object{obj}
						groupKey = key
					}
				}
				if list.NextContinuationToken == nil {
					break
				}
				listInput.ContinuationToken = list.NextContinuationToken
			}
			if !yield(readAlbLogsGroup(ctx, bucketName, group, since, end, s3Client)) {
				return
			}
			// Keep looping next day's logs (resets time)
			since = time.Date(year, month, day+1, 0, 0, 0, 0, time.UTC)
		}
	}
}

type ALBLogEntry struct {
	Message   string
	Timestamp time.Time
}

func readAlbLogsGroup(ctx context.Context, bucketName string, group []s3types.Object, since, end time.Time, s3Client s3Lister) ([]ALBLogEntry, error) {
	var allEntries []ALBLogEntry
	for _, obj := range group {
		content, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: &bucketName,
			Key:    obj.Key,
		})
		if err != nil {
			return nil, err // or continue with other objects?
		}
		entries, err := readAlbLogs(content.Body, since, end)
		if err != nil {
			return nil, err // or continue with other objects?
		}
		if allEntries == nil {
			allEntries = entries
		} else {
			allEntries = append(allEntries, entries...)
		}
	}
	// Always need to sort, because log entries within each object are not in order.
	slices.SortFunc(allEntries, func(a, b ALBLogEntry) int {
		return a.Timestamp.Compare(b.Timestamp)
	})
	return allEntries, nil
}

var errMalformedALBLogLine = errors.New("malformed ALB log line")

func parseAlbLogTime(logLine string) (time.Time, error) {
	// https 2026-02-05T23:58:32.578204Z app/defang-project-stack7d0286/c9b3756e8ef89456 11.22.33.44:34025 - -1 -1 -1 404 - 842 1023 "POST https://11.22.33.44:443/ HTTP/1.1" "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36 Edg/115.0.1901.203" ECDHE-RSA-AES128-GCM-SHA256 TLSv1.2 - "Root=1-69852ea8-7429674e211c223e3c211c6d" "-" "arn:aws:acm:us-test-2:123456789012:certificate/be524858-3414-4e98-be52-240358d85b1c" 0 2026-02-05T23:58:32.493000Z "fixed-response" "-" "-" "-" "-" "-" "-" TID_ba88f3bfb4f5c249b7d9f74348a70697 "-" "-" "-"
	timestampStart := strings.IndexByte(logLine, ' ') + 1 // will be 0 if not found
	timestampEnd := strings.IndexByte(logLine[timestampStart:], ' ') + timestampStart
	if timestampEnd <= timestampStart {
		return time.Time{}, errMalformedALBLogLine
	}
	return time.Parse(time.RFC3339Nano, logLine[timestampStart:timestampEnd])
}

func readAlbLogs(body io.ReadCloser, since, end time.Time) ([]ALBLogEntry, error) {
	defer body.Close()
	gzipReader, err := gzip.NewReader(body)
	if err != nil {
		return nil, err
	}
	var entries []ALBLogEntry
	lineScanner := bufio.NewScanner(gzipReader)
	for lineScanner.Scan() {
		logLine := lineScanner.Text()
		timestamp, err := parseAlbLogTime(logLine)
		if err != nil {
			continue // malformed timestamp: ignore
		}
		if timestamp.Before(since) {
			continue
		}
		if timestamp.After(end) {
			continue // can't break, because there can be out-of-order timestamps
		}
		entries = append(entries, ALBLogEntry{
			Message:   logLine,
			Timestamp: timestamp,
		})
	}
	if err := lineScanner.Err(); err != nil {
		return nil, err
	}
	if err := gzipReader.Close(); err != nil {
		return nil, err // only returns err on failed checksum after io.EOF
	}
	return entries, nil
}
