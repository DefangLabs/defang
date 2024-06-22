package compose

import "testing"

func TestNormalizeServiceName(t *testing.T) {
	testCases := []struct {
		name     string
		expected string
	}{
		{name: "normal", expected: "normal"},
		{name: "camelCase", expected: "camelcase"},
		{name: "PascalCase", expected: "pascalcase"},
		{name: "hyphen-ok", expected: "hyphen-ok"},
		{name: "snake_case", expected: "snake-case"},
		{name: "$ymb0ls", expected: "-ymb0ls"},
		{name: "consecutive--hyphens", expected: "consecutive-hyphens"},
		{name: "hyphen-$ymbol", expected: "hyphen-ymbol"},
		{name: "_blah", expected: "-blah"},
	}
	for _, tC := range testCases {
		t.Run(tC.name, func(t *testing.T) {
			actual := NormalizeServiceName(tC.name)
			if actual != tC.expected {
				t.Errorf("NormalizeServiceName() failed: expected %v, got %v", tC.expected, actual)
			}
		})
	}
}
