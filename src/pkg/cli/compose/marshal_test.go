package compose

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarshalYAML(t *testing.T) {
	p, err := LoadFromContent(t.Context(), []byte(`services:
  service1:
    build: .
    deploy:
      replicas: 3
    environment:
      FLOAT: 1.23
      GITSHA: 65e1234 # too big to fit in 64-bit float
      INTEGER: 1234
      LARGE: 1_000_000_000_000_000_000
      OCTAL: 01234
      OCTALO: 0o1234
      STRING: hello world
`), "TestMarshalYAML")
	require.NoError(t, err)

	b, err := MarshalYAML(p)
	require.NoError(t, err)

	expected := `name: TestMarshalYAML
services:
  service1:
    build:
      context: .
      dockerfile: Dockerfile
    deploy:
      replicas: 3
    environment:
      FLOAT: "1.23"
      GITSHA: "65e1234"
      INTEGER: "1234"
      LARGE: "1000000000000000000"
      OCTAL: "668"
      OCTALO: "668"
      STRING: hello world
    networks:
      default: null
networks:
  default:
    name: TestMarshalYAML_default
`
	require.Equal(t, expected, string(b))
}
