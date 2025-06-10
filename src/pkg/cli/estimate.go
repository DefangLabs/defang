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

func RunEstimate(ctx context.Context, project *compose.Project, client cliClient.FabricClient, previewProvider cliClient.Provider, estimateProvider cliClient.ProviderID, region string, mode defangv1.DeploymentMode) (*defangv1.EstimateResponse, error) {
	preview, err := GeneratePreview(ctx, project, client, previewProvider, mode)
	if err != nil {
		return nil, err
	}

	term.Info("Preparing estimate")

	estimate, err := client.Estimate(ctx, &defangv1.EstimateRequest{
		Provider:      estimateProvider.Value(),
		Region:        region,
		PulumiPreview: []byte(preview),
	})
	if err != nil {
		return nil, err
	}
	return estimate, nil
}

func GeneratePreview(ctx context.Context, project *compose.Project, client client.FabricClient, provider cliClient.Provider, mode defangv1.DeploymentMode) (string, error) {
	os.Setenv("DEFANG_JSON", "1") // always show JSON output for estimate
	since := time.Now()

	resp, project, err := ComposeUp(ctx, project, client, provider, compose.UploadModePreview, mode)
	if err != nil {
		return "", err
	}

	term.Info("Generating deployment preview")
	var pulumiPreviewLogLines []string
	options := TailOptions{
		EndEventDetectFunc: func(services []string, host string, eventlog string) bool {
			return strings.HasPrefix(eventlog, "Preview succeeded") || strings.HasPrefix(eventlog, "Preview failed")
		},
		Deployment: resp.Etag,
		Since:      since,
		LogType:    logs.LogTypeBuild,
		Verbose:    true,
	}

	err = streamLogs(ctx, provider, project.Name, options, func(entry *defangv1.LogEntry, options *TailOptions) error {
		if strings.HasPrefix(entry.Message, "Preview succeeded") || strings.HasPrefix(entry.Message, "Preview failed") {
			return io.EOF
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

var developmentEstimateSummary = `
The development mode is optimized for low cost and rapid iteration. It uses
spot instances and lightweight, burstable resources. Logging is verbose but
short-lived (1 day), deployments are replaced entirely on updates, and
operations that cause downtime are allowed. Ideal for testing and active
development, this mode emphasizes affordability over reliability.`

var stagingEstimateSummary = `
Staging mode strikes a balance between cost and resiliency. It uses similar
infrastructure to production—such as rolling deployments, longer log retention
(7 days), and stable DNS behavior—while still leveraging some cost-saving
measures like spot instances. Suitable for final validation before release.`

var productionEstimateSummary = `
The production environment prioritizes performance, security, and uptime. It
uses on-demand instances, production-grade databases, rolling deployments, and
full networking features like NAT gateways and HTTPS termination protection.
Logs are retained for 30 days, and storage is encrypted. This mode is designed
for serving live traffic reliably.`

func PrintEstimate(mode defangv1.DeploymentMode, estimate *defangv1.EstimateResponse) {
	subtotal := (*money.Money)(estimate.Subtotal)
	tableItems := prepareEstimateLineItemTableItems(estimate.LineItems)
	term.Println("")
	if mode == defangv1.DeploymentMode_DEVELOPMENT || mode == defangv1.DeploymentMode_MODE_UNSPECIFIED {
		term.Println("Development Mode Estimate")
		term.Println(developmentEstimateSummary)
	} else if mode == defangv1.DeploymentMode_STAGING {
		term.Println("Staging Mode Estimate")
		term.Println(stagingEstimateSummary)
	} else if mode == defangv1.DeploymentMode_PRODUCTION {
		term.Println("Production Mode Estimate")
		term.Println(productionEstimateSummary)
	} else {
		term.Printf("Estimate for %s Mode\n", mode.String())
	}

	term.Table(tableItems, []string{"Cost", "Quantity", "Service", "Description"})
	term.Printf("Estimated Monthly Cost: %s (+ usage)\n", subtotal.String())
	term.Println("")
	term.Printf("Estimate does not include taxes or Discount Programs.\n")
	term.Println("")
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
