package aws

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go/ptr"
	"github.com/stretchr/testify/require"
)

func Test_readAlbLogs(t *testing.T) {
	gz, err := os.Open("testdata/123456789012_elasticloadbalancing_us-west-2_app.defang-agentic-strands-aws7d0286.c9b3756e8ef89456_20260206T0000Z_44.233.47.227_7tj887d8.log.gz")
	require.NoError(t, err)
	entries, err := readAlbLogs(gz, time.Time{}, time.Now(), "")
	require.NoError(t, err)
	for _, entry := range entries {
		t.Logf("%s: %s", entry.Timestamp, entry.Message)
	}
}

type mockS3Lister struct{}

func (m mockS3Lister) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	entries, err := os.ReadDir(filepath.Join(".", *params.Bucket))
	contents := make([]s3types.Object, len(entries))
	for i, entry := range entries {
		contents[i].Key = ptr.String(entry.Name())
		contents[i].LastModified = ptr.Time(time.Now())
	}
	return &s3.ListObjectsV2Output{
		Contents: contents,
	}, err
}

func (m mockS3Lister) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	body, err := os.Open(*params.Key)
	return &s3.GetObjectOutput{
		Body: body,
	}, err
}

func Test_streamAlbLogGroup(t *testing.T) {
	s3Client := mockS3Lister{}

	t.Run("empty group", func(t *testing.T) {
		entries, err := readAlbLogsGroup(t.Context(), "testdata", nil, time.Time{}, time.Now(), s3Client, "")
		require.NoError(t, err)
		require.Empty(t, entries)
	})

	t.Run("with test files", func(t *testing.T) {
		files, err := os.ReadDir("testdata")
		require.NoError(t, err)
		var objects []s3types.Object
		for _, f := range files {
			if filepath.Ext(f.Name()) == ".gz" {
				objects = append(objects, s3types.Object{
					Key:          ptr.String(filepath.Join("testdata", f.Name())),
					LastModified: ptr.Time(time.Now()),
				})
			}
		}
		entries, err := readAlbLogsGroup(t.Context(), "testdata", objects, time.Time{}, time.Now(), s3Client, "")
		require.NoError(t, err)
		for _, entry := range entries {
			t.Logf("%s: %s", entry.Timestamp, entry.Message)
		}
		require.NotEmpty(t, entries)
		// Verify entries are sorted by timestamp
		for i := 1; i < len(entries); i++ {
			require.False(t, entries[i].Timestamp.Before(entries[i-1].Timestamp), "entries not sorted at index %d", i)
		}
	})
}
