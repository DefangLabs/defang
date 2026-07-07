package term

import (
	"runtime"
	"time"
)

func Timing() func() {
	var name string
	if pc, _, _, ok := runtime.Caller(1); ok { //nolint:dogsled
		name = runtime.FuncForPC(pc).Name()
	}
	start := time.Now()
	return func() { Debug("timing", time.Since(start), "in", name) }
}
