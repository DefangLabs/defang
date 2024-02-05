// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package term

import (
	"golang.org/x/sys/windows"
)

type state struct {
	mode uint32
}

func makeUnbuf(fd int) (*State, error) {
	var st uint32
	if err := windows.GetConsoleMode(windows.Handle(fd), &st); err != nil {
		return nil, err
	}
	raw := st &^ (windows.ENABLE_ECHO_INPUT | windows.ENABLE_PROCESSED_INPUT | windows.ENABLE_LINE_INPUT)
	if err := windows.SetConsoleMode(windows.Handle(fd), raw); err != nil {
		return nil, err
	}
	return &State{state{st}}, nil
}

func restore(fd int, state *State) error {
	return windows.SetConsoleMode(windows.Handle(fd), state.mode)
}
