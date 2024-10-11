package main

import (
	"bytes"
	"testing"
)

func TestSkipLines(t *testing.T) {
	buf := []byte(`goroutine 1 [running]:
main.main.func1()
        /Users/user/dev/defang/src/cmd/cli/main.go:18 +0x100
panic({0x105f35200?, 0x10670b530?})
        /nix/store/v8llgr5prc0rawmgynacggg0q4pbvk5w-go-1.21.10/share/go/src/runtime/panic.go:914 +0x218
github.com/DefangLabs/defang/src/pkg/clouds/do/appPlatform.newClient({0x106729288, 0x10743e240})
        /Users/user/dev/defang/src/pkg/clouds/do/appPlatform/setup.go:224 +0xb0`)

	expected := []byte(`github.com/DefangLabs/defang/src/pkg/clouds/do/appPlatform.newClient({0x106729288, 0x10743e240})
        /Users/user/dev/defang/src/pkg/clouds/do/appPlatform/setup.go:224 +0xb0`)

	actual := skipLines(buf, 6)
	if !bytes.Equal(expected, actual) {
		t.Errorf("Expected %q, got %q", expected, actual)
	}
}
