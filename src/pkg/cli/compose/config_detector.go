package compose

import (
	"errors"
	"strings"

	"github.com/DefangLabs/secret-detector/pkg/scanner"
)

// assume that the input is a key-value pair string
func detectConfig(input string) (detectorTypes []string, err error) {
	// Detectors check for certain formats in a string to determine if it contains a secret.
	// 		aws_client_id: AWS Client ID
	// 		github: GitHub Personal Access Token
	// 		high_entropy_string: calculated high entropy (randomness) in a string
	// 		keyword: key contains a keyword (e.g. KEY, PASSWORD, SECRET, TOKEN, etc.)
	// 		url_password: passwords in URL format

	// Some detectors allow additional configuration options.
	// "3" is the entropy threshold value (0 = low entropy, 4 = high entropy).

	// create a custom scanner config in json
	jsonCfg := `{
		"detectors": ["aws_client_id", "github", "high_entropy_string", "keyword", "url_password"],
		"detectors_configs": {"keyword": ["3"], "high_entropy_string": ["3"]}
		}`
	cfg, err := scanner.NewConfigFromJson(strings.NewReader(jsonCfg))
	if err != nil {
		return nil, errors.New("Failed to make a config detector: " + err.Error())
	}

	// create a scanner from scanner config
	scannerClient, err := scanner.NewScannerFromConfig(cfg)
	if err != nil {
		return nil, errors.New("Failed to make a config detector: " + err.Error())
	}

	// scan the input
	ds, err := scannerClient.Scan(input)
	if err != nil {
		return nil, errors.New("Failed to scan input: " + err.Error())
	}

	// return a list of detector types
	list := []string{}
	for _, d := range ds {
		list = append(list, d.Type)
	}

	return list, nil
}
