// package compose

package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/DefangLabs/secret-detector/pkg/scanner"
	"github.com/DefangLabs/secret-detector/pkg/secrets"
)

func printScanOutput(ds []secrets.DetectedSecret, err error) {
	fmt.Println("secrets: ")
	for _, d := range ds {
		fmt.Printf("\ttype: %s\n", d.Type)
		fmt.Printf("\tkey: %s\n", d.Key)
		fmt.Printf("\tvalue: %s\n", d.Value)
	}
	fmt.Println("err: ", err)
}

func detectConfig(input string) (detectedSecrets []secrets.DetectedSecret, err error) {
	// note: high entropy and keyword detectors do not work

	// create a custom config for scanner from json
	jsonCfg := `{
		"transformers": ["json"],
		"detectors": ["basic_auth", "high_entropy_string", "keyword", "url_password"],
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

	return ds, err

	// printScanOutput(ds, err)
}

func main() {
	// load config from json
	ds1, err1 := detectConfig("LINK: https://user:p455w0rd@example.com, LINK2: https://user:p483w0rd@example.com")
	if err1 != nil {
		fmt.Println("Error: ", err1)
	}
	printScanOutput(ds1, err)
}

//WORKS FINE
// func main() {

// 	scanner := scanner.NewDefaultScanner()

// 	command := "ENV GITHUB_KEY=ghu_bWIj6excOoiobxoT_g0Ke1BChnXsuH_6UKpr"
// 	ds, err := scanner.ScanStringWithFormat(command, dataformat.Command)

// 	printScanOutput(ds, err)

// }
