package evaluation

import (
	"encoding/json"
	"fmt"
	"os"
)

type EvaluationDataset struct {
	id          string                    `json:"id"`
	name        string                    `json:"name"`
	description string                    `json:"description"`
	inputs      []*EvaluationDatasetInput `json:"inputs"`
}

type EvaluationDatasetInput struct {
	Input     string                 `json:"input"`
	Reference string                 `json:"reference,omitempty"`
	Context   map[string]interface{} `json:"context,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

func LoadEvaluationDatasets(paths []string) ([]EvaluationDataset, error) {
	var datasetGroups []EvaluationDataset
	for _, path := range paths {
		// Read the JSON file
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", path, err)
		}

		// Try to unmarshal as a single datasetGroup first
		var datasetGroup EvaluationDataset
		if err := json.Unmarshal(data, &datasetGroup); err == nil {
			continue
		}

		datasetGroups = append(datasetGroups, datasetGroup)
	}
	return datasetGroups, nil
}
