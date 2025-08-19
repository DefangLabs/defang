package cli

import (
	"encoding/json"

	"github.com/DefangLabs/defang/src/pkg/term"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v3"
)

func MarshalPretty(root string, data proto.Message) ([]byte, error) {
	// HACK: convert to JSON first so we respect the json tags (like "omitempty")
	bytes, err := protojson.Marshal(data)
	if err != nil {
		return nil, err
	}
	var raw map[string]any // TODO: this messes with the order of the fields
	if err := json.Unmarshal(bytes, &raw); err != nil {
		return nil, err
	}
	if root != "" {
		raw = map[string]any{root: raw}
	}
	return yaml.Marshal(raw)
}

func PrintObject(root string, data proto.Message) error {
	bytes, err := MarshalPretty(root, data)
	if err != nil {
		return err
	}
	term.Println(string(bytes))
	return nil
}
