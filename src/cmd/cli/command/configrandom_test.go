package command

import (
	"strings"
	"testing"

	"github.com/DefangLabs/secret-detector/pkg/scanner"
)

func TestCreateRandomConfigValue(t *testing.T) {
	// create the scanner with specific detectors
	// the '3' is the entropy threshold value (0 = low entropy, 4 = high entropy)
	jsonCfg := `{
		"detectors": ["high_entropy_string"],
		"detectors_configs": {"high_entropy_string": ["3"]}
		}`
	cfg, err := scanner.NewConfigFromJson(strings.NewReader(jsonCfg))
	if err != nil {
		t.Fatalf("Failed to make a config detector: " + err.Error())
	}
	scannerClient, err := scanner.NewScannerFromConfig(cfg)
	if err != nil {
		t.Fatalf("Failed to make a config detector: " + err.Error())
	}

	// a map for storing generated results to check if they are unique
	var uniqueConfigList = make(map[string]bool)

	var testIterations = 5
	for range testIterations {
		// call the function to create a random config
		randomConfig := CreateRandomConfigValue()

		// store generated configs as unique keys in a map
		uniqueConfigList[randomConfig] = true

		// scan the config
		ds, err := scannerClient.Scan(randomConfig)
		if err != nil {
			t.Fatalf("Failed to scan input: " + err.Error())
		}

		// the length of ds (detected secrets) should be one
		for _, d := range ds {
			// check if the config meets the threshold for high entropy (randomness)
			if d.Type != "High entropy string" {
				t.Errorf("did not meet the entropy threshold: generated value of %q", randomConfig)
			}
		}
	}

	// check if the length of the map matches the number of test iterations (should be equal if all keys are unique)
	numOfUniqueConfigs := len(uniqueConfigList)
	if numOfUniqueConfigs < testIterations {
		t.Errorf("generated result was not unique: expected numOfUniqueConfigs to be %d, but got %d", testIterations, numOfUniqueConfigs)
	}
}
