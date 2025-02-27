package compose

// package main

import (
	"errors"
	"strings"

	"github.com/DefangLabs/secret-detector/pkg/scanner"
)

// assume that the input is a key-value pair string
func detectConfig(input string) (detectorTypes []string, err error) {
	// create a scanner config in json
	jsonCfg := `{
		"transformers": ["json"],
		"detectors": ["aws_client_id", "github", "high_entropy_string", "keyword", "url_password"],
		"detectors_configs": {"keyword": ["3"], "high_entropy_string": ["3"]},
		"threshold_in_bytes": 1000000}`
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
