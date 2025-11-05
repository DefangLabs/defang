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
		},
	}

	log.Println("Setting up tools.")
	tools := agent.CollectTools(config.Cluster, config.AuthPort, config.ProviderId)
	log.Printf("Tools collected: %d tools", len(tools))

	if len(tools) == 0 {
		log.Fatal("No tools available for the evaluation flow.")
	}

	log.Println("Listing tools:")
	for i, tool := range tools {
		log.Printf("Tool %d: %+v", i, tool)
	}

	log.Println("Initializing runner.")
	r := NewFlowRunner(ctx, config, tools)

	if r == nil {
		log.Fatal("Failed to initialize Flow Runner")
	}

	log.Println("Creating evaluation flow.")
	flow := r.CreateEvaluationFlow()
	log.Printf("Flow created: %+v", flow)

	select {}
}
