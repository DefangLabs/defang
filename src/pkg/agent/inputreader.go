package agent

import (
	"bufio"
	"errors"
	"io"
	"os"
	"os/signal"
	"syscall"
)

// InputReader manages reading from stdin with cancellation support
type InputReader struct {
	scanner   *bufio.Scanner
	inputChan chan string
	errChan   chan error
	sigChan   chan os.Signal
}

// NewInputReader creates a new InputReader that reads from stdin
func NewInputReader() *InputReader {
	ir := &InputReader{
		scanner:   bufio.NewScanner(os.Stdin),
		inputChan: make(chan string),
		errChan:   make(chan error, 1),
		sigChan:   make(chan os.Signal, 1),
	}

	signal.Notify(ir.sigChan, os.Interrupt, syscall.SIGTERM)

	// Start reading in background
	go func() {
		for ir.scanner.Scan() {
			ir.inputChan <- ir.scanner.Text()
		}
		if err := ir.scanner.Err(); err != nil {
			ir.errChan <- err
		}
		close(ir.inputChan)
	}()

	return ir
}

// ReadLine reads the next line of input or returns an error/signal
// Returns (input, nil) on success
// Returns ("", io.EOF) when stdin closes
// Returns ("", ErrInterrupted) when SIGTERM/SIGINT received
// Returns ("", err) on scanner error
func (ir *InputReader) ReadLine() (string, error) {
	select {
	case <-ir.sigChan:
		return "", ErrInterrupted

	case input, ok := <-ir.inputChan:
		if !ok {
			return "", io.EOF
		}
		return input, nil

	case err := <-ir.errChan:
		return "", err
	}
}

// Close stops signal notifications
func (ir *InputReader) Close() {
	signal.Stop(ir.sigChan)
	close(ir.sigChan)
}

var ErrInterrupted = errors.New("interrupted by signal")
