package compose

//package main

import (
	"fmt"

	"github.com/DefangLabs/secret-detector/pkg/dataformat"
	"github.com/DefangLabs/secret-detector/pkg/scanner"
	"github.com/DefangLabs/secret-detector/pkg/secrets"
)

// func main() {

// 	// load config from json
// jsonCfg := `{
// 	"transformers": ["json", "yaml"],
// 	"detectors": ["basic_auth", "high_entropy_string", "keyword", "url_password"],
// 	"threshold_in_bytes": 1000000}`
// cfg, err := scanner.NewConfigFromJson(strings.NewReader(jsonCfg))
// if err != nil {
// 	errors.New("Failed to make a config detector")
// }

// 	// create a scanner
// 	scanner, err := scanner.NewScannerFromConfig(cfg)

// 	//////
// 	// scanner := scanner.NewDefaultScanner()

// 	// // // scanner input can be a file path
// 	// // detectedSecrets, err := scanner.ScanFile("path/to/file")
// 	// // // or an io.Reader
// 	// // var in io.Reader
// 	// // detectedSecrets, err := scanner.ScanReader(in)
// 	// // or just a simple string
// 	// var secrets string = "PASSWORD: hBhwOs2e3m4DsaQ"
// 	detectedSecrets, err := scanner.Scan("PASSWORD: hBhwOs2e3m4DsaQ")

// 	// // print the results
// 	for d := range detectedSecrets {
// 		fmt.Printf("Secret of type '%s' found in '%s'\n", d.Type, d.Key)
// 	}
// }

func printScanOutput(ds []secrets.DetectedSecret, err error) {
	fmt.Println("secrets: ")
	for _, d := range ds {
		fmt.Printf("\ttype: %s\n", d.Type)
		fmt.Printf("\tkey: %s\n", d.Key)
		fmt.Printf("\tvalue: %s\n", d.Value)
	}
	fmt.Println("err: ", err)
}

func main() {

	scanner := scanner.NewDefaultScanner()

	command := "ENV GITHUB_KEY=ghu_bWIj6excOoiobxoT_g0Ke1BChnXsuH_6UKpr"
	ds, err := scanner.ScanStringWithFormat(command, dataformat.Command)

	printScanOutput(ds, err)

}
