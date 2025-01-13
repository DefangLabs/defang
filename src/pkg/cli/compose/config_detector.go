package compose

import (
	"errors"
	"fmt"
	"strings"

	"github.com/DefangLabs/secret-detector/pkg/scanner"
)

func main() {

	// load config from json
	jsonCfg := `{
		"transformers": ["json", "yaml"], 
		"detectors": ["basic_auth", "high_entropy_string", "keyword", "url_password"], 
		"threshold_in_bytes": 1000000}`
	cfg, err := scanner.NewConfigFromJson(strings.NewReader(jsonCfg))
	if err != nil {
		errors.New("Failed to make a config detector")
	}

	// create a scanner
	scanner, err := scanner.NewScannerFromConfig(cfg)

	//////
	// scanner := scanner.NewDefaultScanner()

	// // // scanner input can be a file path
	// // detectedSecrets, err := scanner.ScanFile("path/to/file")
	// // // or an io.Reader
	// // var in io.Reader
	// // detectedSecrets, err := scanner.ScanReader(in)
	// // or just a simple string
	// var secrets string = "PASSWORD: hBhwOs2e3m4DsaQ"
	detectedSecrets, err := scanner.Scan("PASSWORD: hBhwOs2e3m4DsaQ")

	// // print the results
	for d := range detectedSecrets {
		fmt.Printf("Secret of type '%s' found in '%s'\n", d.Type, d.Key)
	}
}
