package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v3"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

var (
	DoDryRun = false

	ErrDryRun = errors.New("dry run")
)

func MarshalPretty(root string, data proto.Message) ([]byte, error) {
	// HACK: convert to JSON first so we respect the json tags (like "omitempty")
	bytes, err := protojson.Marshal(data)
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{} // TODO: this messes with the order of the fields
	if err := json.Unmarshal(bytes, &raw); err != nil {
		return nil, err
	}
	if root != "" {
		raw = map[string]interface{}{root: raw}
	}
	return yaml.Marshal(raw)
}

func PrintObject(root string, data proto.Message) error {
	bytes, err := MarshalPretty(root, data)
	if err != nil {
		return err
	}
	// TODO: add color
	fmt.Println(string(bytes))
	return nil
}

var indentSpaces = "    "

func PrintConfigList(projectname string, configs []*defangv1.ConfigKey) {
	if len(configs) == 0 {
		fmt.Printf("No config to list\n")
		return
	}

	if projectname != "" {
		fmt.Printf("Project: %s\n", projectname)
	}

	fmt.Println("Configs:")
	for _, config := range configs {
		fmt.Printf("%s - %s\n", indentSpaces, config.Name)
	}
}

func PrintConfigData(projectname string, configs []*defangv1.Config) {
	if len(configs) == 0 {
		fmt.Printf("No config values found\n")
		return
	}

	fmt.Printf("Project: %s\n", projectname)
	fmt.Println("Configs:")
	for _, config := range configs {
		if (*config).Sensitivity == defangv1.Sensitivity_SENSITIVE {
			fmt.Printf("%s - %s: [hidden]\n", indentSpaces, config.Name)
		} else {
			fmt.Printf("%s - %s: %s\n", indentSpaces, config.Name, config.Value)
		}
	}
}
