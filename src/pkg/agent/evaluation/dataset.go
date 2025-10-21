package evaluation

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/firebase/genkit/go/ai"
)

// DatasetExample represents a single example in an evaluation dataset
type DatasetExample struct {
	Input     string                 `json:"input"`
	Reference string                 `json:"reference,omitempty"`
	Context   map[string]interface{} `json:"context,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Dataset represents a collection of examples for evaluation
type Dataset struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Examples    []*DatasetExample      `json:"examples"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// DatasetManager handles creation, loading, and management of evaluation datasets
type DatasetManager struct {
	datasets map[string]*Dataset
}

// NewDatasetManager creates a new dataset manager
func NewDatasetManager() *DatasetManager {
	return &DatasetManager{
		datasets: make(map[string]*Dataset),
	}
}

// CreateDataset creates a new empty dataset
func (dm *DatasetManager) CreateDataset(id, name, description string) *Dataset {
	dataset := &Dataset{
		ID:          id,
		Name:        name,
		Description: description,
		Examples:    make([]*DatasetExample, 0),
		Metadata:    make(map[string]interface{}),
	}
	dm.datasets[id] = dataset
	return dataset
}

// AddExample adds an example to a dataset
func (dm *DatasetManager) AddExample(datasetID string, example *DatasetExample) error {
	dataset, exists := dm.datasets[datasetID]
	if !exists {
		return fmt.Errorf("dataset with ID %s not found", datasetID)
	}

	dataset.Examples = append(dataset.Examples, example)
	return nil
}

// GetDataset retrieves a dataset by ID
func (dm *DatasetManager) GetDataset(id string) (*Dataset, error) {
	dataset, exists := dm.datasets[id]
	if !exists {
		return nil, fmt.Errorf("dataset with ID %s not found", id)
	}
	return dataset, nil
}

// LoadDatasetFromFile loads a dataset from a JSON file
func (dm *DatasetManager) LoadDatasetFromFile(filePath string) (*Dataset, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read dataset file %s: %w", filePath, err)
	}

	var dataset Dataset
	if err := json.Unmarshal(data, &dataset); err != nil {
		return nil, fmt.Errorf("failed to parse dataset file %s: %w", filePath, err)
	}

	dm.datasets[dataset.ID] = &dataset
	return &dataset, nil
}

// SaveDatasetToFile saves a dataset to a JSON file
func (dm *DatasetManager) SaveDatasetToFile(datasetID, filePath string) error {
	dataset, exists := dm.datasets[datasetID]
	if !exists {
		return fmt.Errorf("dataset with ID %s not found", datasetID)
	}

	data, err := json.MarshalIndent(dataset, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal dataset: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write dataset file %s: %w", filePath, err)
	}

	return nil
}

// ToGenkitExamples converts dataset examples to Genkit format for evaluation
func (d *Dataset) ToGenkitExamples() []*ai.Example {
	examples := make([]*ai.Example, len(d.Examples))
	for i, example := range d.Examples {
		genkitExample := &ai.Example{
			Input: example.Input,
		}

		if example.Reference != "" {
			genkitExample.Reference = example.Reference
		}

		if example.Context != nil {
			genkitExample.Context = []any{example.Context}
		}

		examples[i] = genkitExample
	}
	return examples
}

// CreateDefangScenarioDataset creates a predefined dataset with common Defang scenarios
func (dm *DatasetManager) CreateDefangScenarioDataset() *Dataset {
	dataset := dm.CreateDataset("defang_scenarios", "Defang Agent Scenarios", "Common scenarios for testing Defang agent")

	// Add example scenarios
	scenarios := []*DatasetExample{
		{
			Input:     "Deploy my application",
			Reference: "(?i)(deploy|deployment)",
			Context:   map[string]interface{}{"scenario": "deploy"},
		},
		{
			Input:     "Show me my deployed services",
			Reference: "(?i)(services|list)",
			Context:   map[string]interface{}{"scenario": "services_list"},
		},
		{
			Input:     "Set a config variable",
			Reference: "(?i)(config|set|variable)",
			Context:   map[string]interface{}{"scenario": "config_set"},
		},
		{
			Input:     "Remove a config variable",
			Reference: "(?i)(config|remove|delete)",
			Context:   map[string]interface{}{"scenario": "config_remove"},
		},
		{
			Input:     "Show me the logs",
			Reference: "(?i)(logs|log)",
			Context:   map[string]interface{}{"scenario": "logs"},
		},
		{
			Input:     "Estimate deployment costs",
			Reference: "(?i)(estimate|cost)",
			Context:   map[string]interface{}{"scenario": "estimate"},
		},
		{
			Input:     "Destroy my deployment",
			Reference: "(?i)(destroy|delete|remove)",
			Context:   map[string]interface{}{"scenario": "destroy"},
		},
		{
			Input:     "Login to Defang",
			Reference: "(?i)(login|authenticate)",
			Context:   map[string]interface{}{"scenario": "login"},
		},
		{
			Input:     "How do I set up AWS provider?",
			Reference: "(?i)(aws|provider|setup)",
			Context:   map[string]interface{}{"scenario": "provider_setup"},
		},
		{
			Input:     "What is my current configuration?",
			Reference: "(?i)(list|show|config)",
			Context:   map[string]interface{}{"scenario": "config_list"},
		},
	}

	for _, scenario := range scenarios {
		dataset.Examples = append(dataset.Examples, scenario)
	}

	return dataset
}

// ListDatasets returns all available datasets
func (dm *DatasetManager) ListDatasets() []*Dataset {
	datasets := make([]*Dataset, 0, len(dm.datasets))
	for _, dataset := range dm.datasets {
		datasets = append(datasets, dataset)
	}
	return datasets
}
