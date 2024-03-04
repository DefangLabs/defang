package local

import (
	"context"
	"testing"
	"time"

	"github.com/defang-io/defang/src/pkg/types"
)

func TestLocal(t *testing.T) {
	l := New()
	ctx := context.TODO()

	t.Run("SetUp", func(t *testing.T) {
		if err := l.SetUp(ctx, []types.Container{{EntryPoint: []string{"/bin/sh"}}}); err != nil {
			t.Fatal(err)
		}
	})
	defer l.TearDown(ctx)

	var pid PID
	t.Run("Run", func(t *testing.T) {
		env := map[string]string{"FOO": "bar"}
		var err error
		pid, err = l.Run(ctx, env, "-c", "sleep 1 ; echo $FOO")
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Tail", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		// This should print "bar" to stdout
		if err := l.Tail(ctx, pid); err != nil {
			t.Error(err)
		}
	})

	t.Run("TearDown", func(t *testing.T) {
		if err := l.TearDown(ctx); err != nil {
			t.Error(err)
		}
	})
}
