package main

import (
	"sync"

	cliClient "github.com/defang-io/defang/src/pkg/cli/client"
)

type P = cliClient.Property // shorthand for tracking properties

// trackWG is used to wait for all tracking to complete.
var trackWG = sync.WaitGroup{}

// track sends a tracking event to the server in a separate goroutine.
func track(name string, props ...cliClient.Property) {
	trackWG.Add(1)
	go func() {
		defer trackWG.Done()
		client.Track(name, props...)
	}()
}

// flushAllTracking waits for all tracking goroutines to complete.
func flushAllTracking() {
	trackWG.Wait()
}
