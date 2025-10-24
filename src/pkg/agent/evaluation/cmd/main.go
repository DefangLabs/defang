package main

import (
	"context"
	"log"

	"github.com/DefangLabs/defang/src/pkg/agent"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/firebase/genkit/go/plugins/evaluators"
)

func main() {
	log.Println("Starting evaluation...")

	ctx := context.Background()
	provider := client.ProviderAWS

	config := FlowConfig{
		Cluster:        "cluster",
		AuthPort:       8080,
		ProviderId:     &provider,
		AIModel:        "googleai/gemini-2.5-flash",
		EvaluationMode: true,
		EvalMetrics: []evaluators.MetricConfig{
			{MetricType: evaluators.EvaluatorRegex},
			{MetricType: evaluators.EvaluatorDeepEqual},
		},
	}

	log.Println("Setting up tools.")
	tools := agent.CollectTools(config.Cluster, config.AuthPort, config.ProviderId)
	log.Println("Tools collected:", len(tools))

	log.Println("Initializing runner.")
	r := NewFlowRunner(ctx, config, tools)

	log.Println("Creating evaluation flow.")
	r.CreateEvaluationFlow()

	// keep the main function running fo developer UI testing
	select {}
}
