package command

import (
	"os"
	"testing"
)

func Test_readGlobals(t *testing.T) {
	t.Chdir("testdata")

	t.Run("OS env beats any .defangrc file", func(t *testing.T) {
		t.Setenv("VALUE", "from OS env")
		readGlobals("test")
		if v := os.Getenv("VALUE"); v != "from OS env" {
			t.Errorf("expected VALUE to be 'from OS env', got '%s'", v)
		}
		os.Unsetenv("VALUE")
	})

	t.Run(".defangrc.test beats .defangrc", func(t *testing.T) {
		readGlobals("test")
		if v := os.Getenv("VALUE"); v != "from .defangrc.test" {
			t.Errorf("expected VALUE to be 'from .defangrc.test', got '%s'", v)
		}
		os.Unsetenv("VALUE")
	})

	t.Run(".defangrc used if no stack", func(t *testing.T) {
		readGlobals("non-existent-stack")
		if v := os.Getenv("VALUE"); v != "from .defangrc" {
			t.Errorf("expected VALUE to be 'from .defangrc', got '%s'", v)
		}
		os.Unsetenv("VALUE")
	})
}
