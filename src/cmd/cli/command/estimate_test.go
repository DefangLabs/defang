package command

import (
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli"
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
	cli.PrintEstimate(modes.ModeAffordable, estimate)

	expectedOutput := `
Estimate for Deployment Mode: AFFORDABLE

This mode is optimized for low cost and rapid iteration. Your application
will deployed with spot instances. Databases will be provisioned using
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
