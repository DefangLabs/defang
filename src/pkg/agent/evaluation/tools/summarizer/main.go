package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	var (
		pathFlag   = flag.String("path", "", "Path to directory containing evaluation JSON files")
		outputFlag = flag.String("output", "", "Output file for summary (optional, prints to stdout if not specified)")
		helpFlag   = flag.Bool("help", false, "Show help message")
	)

	flag.Parse()

	if *helpFlag || *pathFlag == "" {
		fmt.Println("Evaluation Results Summarizer")
		fmt.Println("Usage:")
		fmt.Printf("  %s -path <directory> [-output <file>]\n", os.Args[0])
		fmt.Println()
		fmt.Println("Flags:")
		flag.PrintDefaults()
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Printf("  %s -path ./testrun-branch-commit\n", os.Args[0])
		fmt.Printf("  %s -path ./testrun-branch-commit -output summary.json\n", os.Args[0])

		if *pathFlag == "" && !*helpFlag {
			fmt.Println("\nError: -path flag is required")
			os.Exit(1)
		}
		return
	}

	// Generate summary
	summary := SummarizeEvaluationResults(*pathFlag)

	// Convert to JSON
	summaryJSON, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling summary: %v\n", err)
		os.Exit(1)
	}

	// Output results
	if *outputFlag != "" {
		// Suggest .json extension if not present
		outputFile := *outputFlag
		if !strings.HasSuffix(strings.ToLower(outputFile), ".json") {
			fmt.Printf("Note: Consider using .json extension for JSON output\n")
		}

		err = os.WriteFile(outputFile, summaryJSON, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing to file %s: %v\n", outputFile, err)
			os.Exit(1)
		}
		fmt.Printf("JSON summary written to %s\n", outputFile)
	} else {
		fmt.Println(string(summaryJSON))
	}
}
