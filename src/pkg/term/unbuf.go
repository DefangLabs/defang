// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package term provides support functions for dealing with terminals, as
// commonly found on UNIX systems.
//
// Putting a terminal into raw mode is the most common requirement:
//
//	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
//	if err != nil {
//	        panic(err)
//	}
//	defer term.Restore(int(os.Stdin.Fd()), oldState)
//
// Note that on non-Unix systems os.Stdin.Fd() may not be 0
package term

// State contains the state of a terminal.
type State struct {
	state
}

// MakeRaw puts the terminal connected to the given file descriptor into raw
// mode and returns the previous state of the terminal so that it can be
// restored.
func MakeUnbuf(fd int) (*State, error) {
	return makeUnbuf(fd)
}

// Restore restores the terminal connected to the given file descriptor to a
// previous state.
func Restore(fd int, oldState *State) error {
	return restore(fd, oldState)
}
