package command

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/money"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/spf13/cobra"
)

func makeEstimateCmd() *cobra.Command {
	var estimateCmd = &cobra.Command{
		Use:         "estimate",
		Args:        cobra.NoArgs,
		Annotations: authNeededAnnotation,
		Short:       "Estimate the cost of deploying the current project",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			region, _ := cmd.Flags().GetString("region")

			loader := configureLoader(cmd)
			project, err := loader.LoadProject(ctx)
			if err != nil {
				return err
			}

			provider, err := getProvider(ctx, loader)
			if err != nil {
				return err
			}

			err = canIUseProvider(ctx, provider, project.Name)
			if err != nil {
				return err
			}

			return RunEstimate(ctx, project, provider, region, mode.Value())
		},
	}

	estimateCmd.Flags().VarP(&mode, "mode", "m", fmt.Sprintf("deployment mode; one of %v", allModes()))
	estimateCmd.Flags().StringP("region", "r", pkg.Getenv("AWS_REGION", "us-west-2"), "which cloud region to estimate")
	return estimateCmd
}

func RunEstimate(ctx context.Context, project *compose.Project, provider cliClient.Provider, region string, mode defangv1.DeploymentMode) error {
	term.Info("Generating deployment preview")
	preview, err := GeneratePreview(ctx, project, provider, mode)
	if err != nil {
		return fmt.Errorf("failed to generate preview: %w", err)
	}
	term.Debugf("Preview output: %s\n", preview)

	term.Info("Preparing estimate")
	estimate, err := client.Estimate(ctx, &defangv1.EstimateRequest{
		Provider:      providerID.EnumValue(),
		Region:        region,
		PulumiPreview: []byte(preview),
	})
	if err != nil {
		return fmt.Errorf("failed to estimate: %w", err)
	}

	term.Debugf("Estimate: %+v", estimate)

	PrintEstimate(estimate)

	return nil
}

func GeneratePreview(ctx context.Context, project *compose.Project, provider cliClient.Provider, mode defangv1.DeploymentMode) (string, error) {
	os.Setenv("DEFANG_JSON", "1") // always show JSON output for estimate
	since := time.Now()

	resp, project, err := cli.ComposeUp(ctx, project, client, provider, compose.UploadModePreview, mode)
	if err != nil {
		return "", err
	}

	var pulumiPreviewLogLines []string
	options := cli.TailOptions{
		Deployment: resp.Etag,
		Since:      since,
		LogType:    logs.LogTypeBuild,
		Verbose:    true,
	}

	err = cli.TailAndWaitForCD(ctx, project.Name, provider, options, func(entry *defangv1.LogEntry, options *cli.TailOptions) error {
		pulumiPreviewLogLines = append(pulumiPreviewLogLines, entry.GetMessage())
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to tail and wait for cd: %w", err)
	}

	return strings.Join(pulumiPreviewLogLines, "\n"), nil
}

func PrintEstimate(estimate *defangv1.EstimateResponse) {
	subtotal := (*money.Money)(estimate.Subtotal)
	tableItems := prepareEstimateLineItemTableItems(estimate.LineItems)
	term.Table(tableItems, []string{"Cost", "Quantity", "Description"})
	term.Printf("Estimated Monthly Cost: %s (+ usage)\n", subtotal.String())
	term.Printf("Estimate does not include taxes or Discount Programs.\n")
}

type EstimateLineItemTableItem struct {
	Cost        string
	Quantity    string
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
			Description: lineItem.Description,
		}
	}

	// sort line items by description
	sort.Slice(tableItems, func(i, j int) bool {
		return tableItems[i].Description < tableItems[j].Description
	})

	return tableItems
}
