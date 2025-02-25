package compose

// package main

import (
	"errors"
	"strings"

	"github.com/DefangLabs/secret-detector/pkg/scanner"
)

// func printScanOutput(ds []secrets.DetectedSecret, err error) {
// 	fmt.Println("secrets: ")
// 	for _, d := range ds {
// 		fmt.Printf("\ttype: %s\n", d.Type)
// 		fmt.Printf("\tkey: %s\n", d.Key)
// 		fmt.Printf("\tvalue: %s\n", d.Value)
// 	}
// 	fmt.Println("err: ", err)
// }

// assume that the input is the value of a key-value pair

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

	ds, err := scannerClient.Scan(input)
	if err != nil {
		return nil, errors.New("Failed to scan input: " + err.Error())
	}

	if len(ds) == 0 {
		return nil, errors.New("no secrets detected")
	}

	list := []string{}
	for _, d := range ds {
		list = append(list, d.Type)
	}

	return list, nil
	// printScanOutput(ds, err)
}

// func main() {
// 	// load config from json
// 	ds1, err1 := detectConfig("basic dTpw")
// 	if err1 != nil {
// 		fmt.Println("Error: ", err1)
// 	}
// 	printScanOutput(ds1, err1)
// }

//WORKS FINE
// func main() {

// 	scanner := scanner.NewDefaultScanner()

// 	command := "ENV GITHUB_KEY=ghu_bWIj6excOoiobxoT_g0Ke1BChnXsuH_6UKpr"
// 	ds, err := scanner.ScanStringWithFormat(command, dataformat.Command)

// 	printScanOutput(ds, err)

// }
