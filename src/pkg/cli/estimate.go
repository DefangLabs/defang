package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/money"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func RunEstimate(ctx context.Context, project *compose.Project, client cliClient.FabricClient, previewProvider cliClient.Provider, estimateProviderID cliClient.ProviderID, region string, mode defangv1.DeploymentMode) (*defangv1.EstimateResponse, error) {
	term.Debugf("Running estimate for project %s in region %s with mode %s", project.Name, region, mode)
	preview, err := GeneratePreview(ctx, project, client, previewProvider, mode)
	if err != nil {
		return nil, err
	}

	term.Info("Preparing estimate")

	estimate, err := client.Estimate(ctx, &defangv1.EstimateRequest{
		Provider:      estimateProviderID.Value(),
		Region:        region,
		PulumiPreview: []byte(preview),
	})
	if err != nil {
		return nil, err
	}
	return estimate, nil
}

func GeneratePreview(ctx context.Context, project *compose.Project, client client.FabricClient, provider cliClient.Provider, mode defangv1.DeploymentMode) (string, error) {
	os.Setenv("DEFANG_JSON", "1") // HACK: always show JSON output for estimate
	since := time.Now()

	resp, project, err := ComposeUp(ctx, project, client, provider, compose.UploadModeEstimate, mode)
	if err != nil {
		return "", err
	}

	term.Info("Generating deployment preview")
	var pulumiPreviewLogLines []string
	options := TailOptions{
		Deployment: resp.Etag,
		Since:      since,
		LogType:    logs.LogTypeBuild,
		Verbose:    true,
	}

	err = streamLogs(ctx, provider, project.Name, options, func(entry *defangv1.LogEntry, options *TailOptions) error {
		if strings.HasPrefix(entry.Message, "Preview succeeded") {
			return io.EOF
		} else if strings.HasPrefix(entry.Message, "Preview failed") {
			return errors.New(entry.Message)
		}
		term.Debug(entry.Message)
		pulumiPreviewLogLines = append(pulumiPreviewLogLines, entry.Message)
		return nil
	})
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("failed to tail and wait for cd: %w", err)
	}

	return strings.Join(pulumiPreviewLogLines, "\n"), nil
}

var affordableModeEstimateSummary = `
This mode is optimized for low cost and rapid iteration. Your application
will deployed with spot instances. Databases will be provisioned using
resources optimized for burstable memory. Deployments are replaced entirely on
updates, so there may be small windows of downtime during redeployment.
Services will be exposed directly to the public internet for easy debugging.
This mode emphasizes affordability over availability.`

var balancedModeEstimateSummary = `
This mode strikes a balance between cost and availability. Your application
will be deployed with spot instances. Databases will be provisioned using
resources optimized for production. Services in the "internal" network will
be deployed to a private subnet with a NAT gateway for outbound internet access.`

var highAvailabilityModeEstimateSummary = `
This mode prioritizes availability. Your application
will deployed with on-demand instances in multiple availability zones.
Databases will be provisioned using resources optimized for production.
Services in the "internal" network will be deployed to a private subnet with a
NAT gateway for outbound internet access.`

func PrintEstimate(mode defangv1.DeploymentMode, estimate *defangv1.EstimateResponse) {
	subtotal := (*money.Money)(estimate.Subtotal)
	tableItems := prepareEstimateLineItemTableItems(estimate.LineItems)
	term.Println("")
	if mode == defangv1.DeploymentMode_DEVELOPMENT || mode == defangv1.DeploymentMode_MODE_UNSPECIFIED {
		term.Println("Estimate for Deployment Mode: AFFORDABLE")
		term.Println(affordableModeEstimateSummary)
	} else if mode == defangv1.DeploymentMode_STAGING {
		term.Println("Estimate for Deployment Mode: BALANCED")
		term.Println(balancedModeEstimateSummary)
	} else if mode == defangv1.DeploymentMode_PRODUCTION {
		term.Println("Estimate for Deployment Mode: HIGH_AVAILABILITY")
		term.Println(highAvailabilityModeEstimateSummary)
	} else {
		term.Printf("Estimate for %s Mode\n", mode.String())
	}

	term.Table(tableItems, []string{"Cost", "Quantity", "Service", "Description"})
	term.Printf("Estimated Monthly Cost: %s (+ usage)\n", subtotal.String())
	term.Println("")
	term.Printf("Estimate does not include taxes or Discount Programs.\n")
	term.Println("To estimate other modes, use defang estimate --mode=affordable|balanced|high_availability")
}

type EstimateLineItemTableItem struct {
	Cost        string
	Quantity    string
	Service     string
	Description string
}

func prepareEstimateLineItemTableItems(lineItems []*defangv1.EstimateLineItem) []EstimateLineItemTableItem {
	tableItems := make([]EstimateLineItemTableItem, len(lineItems))
	for i, lineItem := range lineItems {
		cost := (*money.Money)(lineItem.Cost)
		var quantityStr string
		if lineItem.Quantity == float32(int(lineItem.Quantity)) {
			quantityStr = strconv.Itoa(int(lineItem.Quantity))
		} else {
			quantityStr = fmt.Sprintf("%.2f", lineItem.Quantity)
		}

		tableItems[i] = EstimateLineItemTableItem{
			Cost:        cost.String(),
			Quantity:    fmt.Sprintf("%s %s", quantityStr, lineItem.Unit),
			Service:     strings.Join(lineItem.Service, ", "),
			Description: lineItem.Description,
		}
	}

	// sort line items by service + description
	sort.Slice(tableItems, func(i, j int) bool {
		return tableItems[i].Service+tableItems[i].Description < tableItems[j].Service+tableItems[j].Description
	})

	return tableItems
}
