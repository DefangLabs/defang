package compose

import (
	"fmt"

	"github.com/DefangLabs/secret-detector/pkg/scanner"
)

// assume that the input is a key-value pair string
func detectConfig(input string) (detectorTypes []string, err error) {
	// Detectors check for certain formats in a string to determine if it contains a secret.
	// Some detectors allow additional configuration options, such as:
	// 		keyword: key contains a keyword (e.g. KEY, PASSWORD, SECRET, TOKEN, etc.)
	// 		high_entropy_string: calculated high entropy (randomness) in a string
	// These detectors require an entropy threshold value (0 = low entropy, 4+ = very high entropy).

	// create a custom scanner config
	cfg := scanner.NewConfigWithDefaults()
	cfg.Transformers = []string{"json"}
	cfg.DetectorConfigs["keyword"] = []string{"3"}
	cfg.DetectorConfigs["high_entropy_string"] = []string{"4"}

	// create a scanner from scanner config
	scannerClient, err := scanner.NewScannerFromConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to make a config detector: %w", err)
	}

	// scan the input
	ds, err := scannerClient.Scan(input)
	if err != nil {
		return nil, fmt.Errorf("failed to scan input: %w", err)
	}

	// return a list of detector types
	list := []string{}
	for _, d := range ds {
		list = append(list, d.Type)
	}

	return list, nil
}
