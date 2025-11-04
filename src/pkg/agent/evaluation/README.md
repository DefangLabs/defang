# Defang CLI Agent Evaluation System

## Overview

The Defang CLI Agent leverages the Firebase GenKit framework to provide agentic tool capabilities and comprehensive evaluation workflows. This system enables systematic testing and validation of Large Language Model (LLM) performance with tool usage scenarios.

The CLI Agent and Model Context Protocol (MCP) implementations share the same underlying tools, ensuring consistency across different interfaces.

## Architecture

### Core Components

- **GenKit Framework**: Provides the evaluation infrastructure and AI workflow management
- **CLI Agent**: The primary interface that users interact with for Defang operations  
- **MCP Integration**: Alternative protocol interface using the same tool implementations
- **Evaluation Pipeline**: Automated testing system for validating LLM and tool performance

### Directory Structure

```text
src/pkg/agent/evaluation/
├── cmd/                    # GenKit evaluation runner (main.go, runner.go)
├── datasets/               # Test cases with input scenarios and expected outcomes
├── tools/
│   └── summarizer/         # Results analysis and reporting tools
└── README.md               # This documentation
```

## Purpose and Use Cases

The evaluation system serves several critical functions:

### Performance Validation

- **LLM Model Changes**: Test new language models or model parameter adjustments
- **Tool Modifications**: Validate updates to tool names, descriptions, or implementations
- **System Updates**: Ensure changes don't regress existing functionality

### Quality Assurance

- **Regression Testing**: Detect performance degradation across system updates
- **Comparative Analysis**: Benchmark different configurations against baseline performance
- **Continuous Monitoring**: Track system performance over time

## Evaluation Workflow

### Prerequisites

- GenKit framework installed and configured
- Go development environment (Go 1.24+)
- Access to the evaluation datasets and test scenarios

### Step-by-Step Process

#### 1. Implement Changes

Make your intended modifications to:

- LLM model selection or configuration
- Tool names, descriptions, or implementations
- Agent behavior or response patterns

#### 2. Start Evaluation Infrastructure

The evaluation system requires a two-terminal setup for optimal workflow:

**Terminal 1 (Server Management):**

```bash
cd src
make genkit-start
```

This starts the GenKit server and keeps it running throughout the evaluation process. The server handles AI workflow execution and maintains evaluation state.

**Terminal 2 (Evaluation Execution):**

```bash
cd src  
make genkit-evaluate
```

This executes the complete evaluation suite against all configured datasets. The process includes:

- Loading test scenarios from the datasets directory
- Running multiple evaluation rounds for statistical reliability
- Generating comprehensive performance reports

#### 3. Analyze Results

**Primary Output File:**

```text
src/pkg/agent/evaluation/current_evaluation.json
```

This file contains:

- **Overall Performance Metrics**: Success rates, response quality scores
- **Tool Usage Analysis**: Which tools were called and how effectively
- **Error Classification**: Types and frequencies of failures
- **Comparative Data**: Performance relative to baseline measurements

**Analysis Commands:**

```bash
# Compare against main branch baseline
git diff main -- src/pkg/agent/evaluation/current_evaluation.json

# View detailed evaluation results
cat src/pkg/agent/evaluation/current_evaluation.json | jq .
```

#### 4. Decision Making

**Performance Improvement Criteria:**

- Equal or higher success rates compared to baseline
- Improved tool selection accuracy
- Reduced error rates or better error handling
- Maintained or improved response quality metrics

**Integration Process:**

If results meet or exceed baseline performance:

1. Commit the `current_evaluation.json` file with your changes
2. Update documentation to reflect any behavioral changes
3. Consider the changes validated for production use

If results show regression:

1. Analyze specific failure modes using detailed logs
2. Refine your changes based on evaluation feedback
3. Re-run evaluation cycle until performance meets standards

## Configuration and Customization

### Evaluation Parameters

- **Run Count**: Configure number of evaluation rounds per dataset
- **Output Formats**: Customize result file naming and location
- **Dataset Selection**: Enable/disable specific test scenarios

### Advanced Usage

```bash
# Custom run count (default: 10 runs per dataset)
make genkit-evaluate RUNS=25

# Custom output file location (default: current_evaluation.json)
make genkit-evaluate EVAL_SUMMARY_FILE=my_evaluation_results.json

# View available configuration options
make genkit-help
```

## Troubleshooting

### Common Issues

- **Server Connection Failures**: Ensure `make genkit-start` is running in separate terminal
- **Evaluation Timeouts**: Check system resources and consider reducing RUNS parameter
- **Dataset Loading Errors**: Verify dataset files are valid JSON format

### Recovery Procedures

```bash
# Restart server if evaluations fail
make genkit-restart

# Clean previous evaluation results
make genkit-clean

# Check server status
make genkit-check-server
```

## Best Practices

### Development Workflow

1. Always run baseline evaluation before making changes
2. Make incremental changes and evaluate frequently
3. Maintain detailed notes about configuration changes
4. Commit evaluation results with corresponding code changes

### Performance Monitoring

- Track evaluation results over time to identify trends
- Compare results across different development branches
- Use statistical analysis for multi-run evaluation data
- Document significant performance changes in commit messages

## Integration with CI/CD

The evaluation system can be integrated into continuous integration pipelines to automatically validate changes before deployment. Consider implementing evaluation runs as part of pull request validation workflows.
