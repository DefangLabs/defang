package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v3"

	"github.com/DefangLabs/defang/src/pkg/types"
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

func PrintConfigData(config types.ConfigData) {
	for key, value := range config {
		if value.IsSensitive {
			fmt.Printf("%s: [hidden]\n", key)
		} else {
			fmt.Printf("%s: %s\n", key, value)
		}
	}
}
