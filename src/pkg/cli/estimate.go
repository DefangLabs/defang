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
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/money"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func RunEstimate(ctx context.Context, project *compose.Project, client client.FabricClient, previewProvider client.Provider, estimateProviderID client.ProviderID, region string, mode modes.Mode) (*defangv1.EstimateResponse, error) {
	term.Debugf("Running estimate for project %s in region %s with mode %s", project.Name, region, mode)
	preview, err := GeneratePreview(ctx, project, client, previewProvider, estimateProviderID, mode, region)
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

func GeneratePreview(ctx context.Context, project *compose.Project, client client.FabricClient, previewProvider client.Provider, estimateProviderID client.ProviderID, mode modes.Mode, region string) (string, error) {
	os.Setenv("DEFANG_JSON", "1")             // HACK: always show JSON output for estimate
	since := time.Now().Add(-1 * time.Minute) // fetch logs since one minute ago to account for clock drift

	fixedProject := project.WithoutUnnecessaryResources()
	if err := compose.FixupServices(ctx, previewProvider, fixedProject, compose.UploadModeEstimate); err != nil {
		return "", err
	}

	composeData, err := fixedProject.MarshalYAML()
	if err != nil {
		return "", fmt.Errorf("failed to marshal compose project: %w", err)
	}

	term.Debugf("Fixedup project: %s", string(composeData))

	resp, err := client.Preview(ctx, &defangv1.PreviewRequest{
		Provider:    estimateProviderID.Value(),
		Mode:        mode.Value(),
		Region:      region,
		Compose:     composeData,
		ProjectName: project.Name,
	})
	if err != nil {
		return "", err
	}

	term.Info("Generating deployment preview, this may take a few minutes...")
	var pulumiPreviewLogLines []string
	tailOptions := TailOptions{
		Deployment: resp.Etag,
		LogType:    logs.LogTypeBuild,
		Since:      since,
		Verbose:    true,
	}

	err = streamLogs(ctx, previewProvider, project.Name, tailOptions, func(entry *defangv1.LogEntry, options *TailOptions, t *term.Term) error {
		if strings.HasPrefix(entry.Message, "Preview succeeded") {
			return io.EOF
		} else if strings.HasPrefix(entry.Message, "Preview failed") {
			return errors.New(entry.Message)
		}
		t.Debug(entry.Message)
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

func PrintEstimate(mode modes.Mode, estimate *defangv1.EstimateResponse, term *term.Term) {
	subtotal := (*money.Money)(estimate.Subtotal)
	tableItems := prepareEstimateLineItemTableItems(estimate.LineItems)
	term.Println("")
	switch mode {
	case modes.ModeAffordable:
		term.Println("Estimate for Deployment Mode: AFFORDABLE")
		term.Println(affordableModeEstimateSummary)
	case modes.ModeBalanced:
		term.Println("Estimate for Deployment Mode: BALANCED")
		term.Println(balancedModeEstimateSummary)
	case modes.ModeHighAvailability:
		term.Println("Estimate for Deployment Mode: HIGH_AVAILABILITY")
		term.Println(highAvailabilityModeEstimateSummary)
	default:
		panic("unexpected mode")
	}

	term.Table(tableItems, "Cost", "Quantity", "Service", "Description")
	term.Printf("Estimated Monthly Cost: %s (+ usage)\n", subtotal.String())
	term.Println("")
	term.Printf("Estimate does not include taxes or Discount Programs.\n")
	term.Printf("To estimate other modes, use defang estimate --mode=%s\n", strings.Join(modes.AllDeploymentModes(), "|"))
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
