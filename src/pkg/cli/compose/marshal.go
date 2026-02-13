package compose

import (
	"bytes"
	"regexp"

	"go.yaml.in/yaml/v3"
)

func MarshalYAML(p *Project) ([]byte, error) {
	// Step 1: Struct -> yaml.Node
	var root yaml.Node
	if err := root.Encode(p); err != nil {
		return nil, err
	}

	// Step 2: Force-quote strings that look like numbers
	quoteAmbiguousStringValues(&root)

	// Step 3: yaml.Node -> []byte (copied from compose-go MarshalYAML)
	buf := bytes.NewBuffer([]byte{})
	encoder := yaml.NewEncoder(buf)
	encoder.SetIndent(2)
	// encoder.CompactSeqIndent() FIXME https://github.com/go-yaml/yaml/pull/753
	// src := applyMarshallOptions(p, options...)
	if err := encoder.Encode(&root); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// This is the regex used by nodeca/js-yaml and eemeli/yaml to determine if a string is a number
var yamlNumberRegex = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]*)?(?:[eE][-+]?[0-9]+)?$`)

func looksLikeNumber(s string) bool {
	return yamlNumberRegex.MatchString(s)
}

func quoteAmbiguousStringValues(node *yaml.Node) {
	if node.Kind == yaml.ScalarNode && node.Style == 0 && node.Tag == "!!str" {
		if looksLikeNumber(node.Value) {
			// We got a string that looks like a number, so force it to be quoted in the output YAML
			node.Style = yaml.DoubleQuotedStyle
		}
	}
	for _, child := range node.Content {
		quoteAmbiguousStringValues(child)
	}
}
