package command

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	_type "github.com/DefangLabs/defang/src/protos/google/type"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/andreyvit/diff"
)

func TestPrintEstimate(t *testing.T) {
	// Test with a sample estimate
	estimate := &defangv1.EstimateResponse{
		Subtotal: &_type.Money{
			CurrencyCode: "USD",
			Units:        118,
			Nanos:        550_000_000,
		},
		LineItems: []*defangv1.EstimateLineItem{
			{
				Service:     []string{"shared"},
				Description: "AWSELB USW2-LoadBalancerUsage",
				Unit:        "Hours",
				Quantity:    730,
				Cost: &_type.Money{
					CurrencyCode: "USD",
					Units:        16,
					Nanos:        430_000_000,
				},
			},
			{
				Service:     []string{"shared"},
				Description: "AmazonEC2 USW2-NatGateway-Hours",
				Unit:        "Hours",
				Quantity:    730,
				Cost: &_type.Money{
					CurrencyCode: "USD",
					Units:        32,
					Nanos:        850_000_000,
				},
			},
			{
				Service:     []string{"app"},
				Description: "AmazonECS USW2-Fargate-EphemeralStorage-GB-Hours (20 GB * 730 hours)",
				Unit:        "GB-Hours",
				Quantity:    14600,
				Cost: &_type.Money{
					CurrencyCode: "USD",
					Units:        1,
					Nanos:        620_000_000,
				},
			},
			{
				Service:     []string{"app"},
				Description: "AmazonECS USW2-Fargate-GB-Hours (2 GB * 730 hours)",
				Unit:        "GB-Hours",
				Quantity:    1460,
				Cost: &_type.Money{
					CurrencyCode: "USD",
					Units:        6,
					Nanos:        490_000_000,
				},
			},
			{
				Service:     []string{"app"},
				Description: "AmazonECS USW2-Fargate-GB-Hours-SpotDiscount (Estimated @ 70%)",
				Unit:        "GB-Hours",
				Quantity:    1460,
				Cost: &_type.Money{
					CurrencyCode: "USD",
					Units:        -4,
					Nanos:        -540_000_000,
				},
			},
			{
				Service:     []string{"app"},
				Description: "AmazonECS USW2-Fargate-vCPU-Hours:perCPU (1.00 vCPU * 730 hours)",
				Unit:        "vCPU-Hours",
				Quantity:    730,
				Cost: &_type.Money{
					CurrencyCode: "USD",
					Units:        29,
					Nanos:        550_000_000,
				},
			},
			{
				Service:     []string{"app"},
				Description: "AmazonECS USW2-Fargate-vCPU-Hours:perCPU-SpotDiscount (Estimated @ 70%)",
				Unit:        "GB-Hours",
				Quantity:    730,
				Cost: &_type.Money{
					CurrencyCode: "USD",
					Units:        -20,
					Nanos:        -690_000_000,
				},
			},
			{
				Service:     []string{"cache"},
				Description: "AmazonElastiCache USW2-NodeUsage:cache.t3.medium",
				Unit:        "%Utilized/mo",
				Quantity:    730,
				Cost: &_type.Money{
					CurrencyCode: "USD",
					Units:        49,
					Nanos:        640_000_000,
				},
			},
			{
				Service:     []string{"db"},
				Description: "AmazonRDS USW2-InstanceUsage:db.t3.medium",
				Unit:        "%Utilized/mo",
				Quantity:    100,
				Cost: &_type.Money{
					CurrencyCode: "USD",
					Units:        7,
					Nanos:        200_000_000,
				},
			},
		},
	}

	stdout, _ := term.SetupTestTerm(t)
	cli.PrintEstimate(modes.RecipeAffordable, estimate, term.DefaultTerm)

	expectedOutput := `
Estimate for Deployment Mode: AFFORDABLE

Available on all tiers.
This mode is optimized for low cost and rapid iteration. Your application
will be deployed with spot instances. Databases will be provisioned using
resources optimized for burstable memory. Deployments are replaced entirely on
updates, so there may be small windows of downtime during redeployment.
Services will be exposed directly to the public internet for easy debugging.
This mode emphasizes affordability over availability.

COST     QUANTITY          SERVICE  DESCRIPTION
$1.62    14600 GB-Hours    app      AmazonECS USW2-Fargate-EphemeralStorage-GB-Hours (20 GB * 730 hours)
$6.49    1460 GB-Hours     app      AmazonECS USW2-Fargate-GB-Hours (2 GB * 730 hours)
-$4.54   1460 GB-Hours     app      AmazonECS USW2-Fargate-GB-Hours-SpotDiscount (Estimated @ 70%)
$29.55   730 vCPU-Hours    app      AmazonECS USW2-Fargate-vCPU-Hours:perCPU (1.00 vCPU * 730 hours)
-$20.69  730 GB-Hours      app      AmazonECS USW2-Fargate-vCPU-Hours:perCPU-SpotDiscount (Estimated @ 70%)
$49.64   730 %Utilized/mo  cache    AmazonElastiCache USW2-NodeUsage:cache.t3.medium
$7.20    100 %Utilized/mo  db       AmazonRDS USW2-InstanceUsage:db.t3.medium
$16.43   730 Hours         shared   AWSELB USW2-LoadBalancerUsage
$32.85   730 Hours         shared   AmazonEC2 USW2-NatGateway-Hours
Estimated Monthly Cost: $118.55 (+ usage)

Estimate does not include taxes or Discount Programs.
To estimate other modes, use defang estimate --mode=AFFORDABLE|BALANCED|HIGH_AVAILABILITY
`

	outputLines := strings.Split(term.StripAnsi(stdout.String()), "\n")
	// for each line remove trailing spaces
	for i, line := range outputLines {
		outputLines[i] = strings.TrimRight(line, " ")
	}

	actualOutput := strings.Join(outputLines, "\n")

	if actualOutput != expectedOutput {
		t.Errorf("Expected output did not match actual output. diff:\n%s", diff.LineDiff(expectedOutput, actualOutput))
	}
}

type regionProvider struct {
	client.Provider
	info *client.AccountInfo
	err  error
}

func (p regionProvider) AccountInfo(context.Context) (*client.AccountInfo, error) {
	return p.info, p.err
}

func TestResolvedPreviewRegion(t *testing.T) {
	t.Setenv("AWS_DEFAULT_REGION", "eu-west-1") // must not be silently replaced by the static default
	tests := []struct {
		name    string
		p       regionProvider
		want    string
		wantErr bool
	}{
		{"uses provider's resolved region", regionProvider{info: &client.AccountInfo{Region: "eu-west-1"}}, "eu-west-1", false},
		{"errors when AccountInfo fails", regionProvider{err: errors.New("sts timeout")}, "", true},
		{"errors on empty region", regionProvider{info: &client.AccountInfo{}}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolvedPreviewRegion(t.Context(), tt.p)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("got (%q, %v), want (%q, err=%v)", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestPickEstimateRegion(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	os.Unsetenv("AWS_REGION") // restored by t.Setenv cleanup
	tests := []struct {
		name        string
		flag        string
		explicit    bool
		stackRegion string
		want        string
		wantErr     bool
	}{
		{"explicit matches", "eu-west-1", true, "eu-west-1", "eu-west-1", false},
		{"explicit mismatch", "us-west-2", true, "eu-west-1", "", true},
		{"explicit, unknown stack region", "ap-south-1", true, "", "ap-south-1", false},
		{"omitted uses stack region", "us-west-2", false, "eu-west-1", "eu-west-1", false},
		{"omitted, no stack region uses provider default", "westus", false, "", client.RegionDefaultAWS, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pickEstimateRegion(tt.flag, tt.explicit, tt.stackRegion, client.ProviderAWS)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("got (%q, %v), want (%q, err=%v)", got, err, tt.want, tt.wantErr)
			}
		})
	}
}
