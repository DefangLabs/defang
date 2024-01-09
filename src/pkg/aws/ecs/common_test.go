package ecs

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestPlatformToArch(t *testing.T) {
	tests := []struct {
		platform string
		want     *string
	}{
		{"", nil},
		{"blah", aws.String("BLAH")}, // invalid platform
		{"amd64", aws.String("X86_64")},
		{"arm64", aws.String("ARM64")},
		{"linux/amd64", aws.String("X86_64")},
		{"linux/arm64", aws.String("ARM64")},
		{"linux/arm64/v8", aws.String("ARM64")},
		{"linux/blah", aws.String("BLAH")}, // invalid platform
	}
	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			if got := PlatformToArch(tt.platform); got == nil && tt.want != nil {
				t.Errorf("PlatformToArch() = nil, want %v", tt.want)
			} else if got != nil && tt.want == nil {
				t.Errorf("PlatformToArch() = %v, want nil", *got)
			} else if got != nil && tt.want != nil && *got != *tt.want {
				t.Errorf("PlatformToArch() = %v, want %v", *got, *tt.want)
			}
		})
	}
}

func TestGetAccountID(t *testing.T) {
	a := AwsEcs{
		TaskDefARN: "arn:aws:ecs:us-east-1:123456789012:task-definition/defang-ecs-2021-08-31-163042",
	}
	if got := a.GetAccountID(); got != "123456789012" {
		t.Errorf("GetAccountID() = %v, want 123456789012", got)
	}
}
