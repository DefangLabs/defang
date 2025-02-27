package compose

// package main

import (
	"errors"
	"strings"

	"github.com/DefangLabs/secret-detector/pkg/scanner"
)

// assume that the input is a key-value pair string
func detectConfig(input string) (detectorTypes []string, err error) {
	// create a custom config for scanner from json
	jsonCfg := `{
		"transformers": ["json"],
		"detectors": ["basic_auth", "high_entropy_string", "keyword", "url_password"],
		"detectors_configs": {"keyword": ["3"], "high_entropy_string": ["3"]},
		"threshold_in_bytes": 1000000}`
	cfg, err := scanner.NewConfigFromJson(strings.NewReader(jsonCfg))
	if err != nil {
		return nil, errors.New("Failed to make a config detector: " + err.Error())
	}
	// create a scanner
	scannerClient, err := scanner.NewScannerFromConfig(cfg)
	if err != nil {
		return nil, errors.New("Failed to make a config detector: " + err.Error())
	}

	// scan the input
	ds, err := scannerClient.Scan(input)
	if err != nil {
		return nil, errors.New("Failed to scan input: " + err.Error())
	}

	// // check if there are any secrets detected
	// if len(ds) == 0 {
	// 	return nil, errors.New("no secrets detected")
	// }

	// return a list of detector types
	list := []string{}
	for _, d := range ds {

		list = append(list, d.Type)
	}

	return list, nil
}
