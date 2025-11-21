package aws

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go/ptr"
)

type mockS3Client struct {
	S3Client
}

func (mockS3Client) GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return &s3.GetObjectOutput{
		Body: io.NopCloser(strings.NewReader(`{
	"version": 3,
	"checkpoint": {
		"latest": {
			"resources": [{}]
		}
	}
}`)),
	}, nil
}

func (mockS3Client) ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	return &s3.ListObjectsV2Output{
		Contents: []types.Object{
			{
				Key:  ptr.String(".pulumi/stacks/project1/stack1.json"),
				Size: ptr.Int64(1500),
			},
			{
				Key:  ptr.String(".pulumi/stacks/project1/stack2.json"),
				Size: ptr.Int64(500), // too small, should be skipped
			},
			{
				Key:  ptr.String(".pulumi/stacks/project2/stack3.json"),
				Size: ptr.Int64(2000),
			},
			{
				Key:  ptr.String(".pulumi/stacks/project2/stack4.bak"), // wrong extension, should be skipped
				Size: ptr.Int64(2000),
			},
		},
	}, nil
}

func TestListPulumiStacks(t *testing.T) {
	client := mockS3Client{}
	stacks, err := ListPulumiStacks(t.Context(), client, "my-bucket")
	if err != nil {
		t.Fatalf("ListPulumiStacks returned error: %v", err)
	}
	expectedStacks := []string{
		"project1/stack1",
		"project2/stack3",
	}
	count := 0
	for stack := range stacks {
		if stack != expectedStacks[count] {
			t.Errorf("expected stack %q, got %q", expectedStacks[count], stack)
		}
		count++
	}
	if count != len(expectedStacks) {
		t.Fatalf("expected %d stacks, got %d", len(expectedStacks), count)
	}
}
