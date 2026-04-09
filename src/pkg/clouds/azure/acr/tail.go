package acr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const logPollInterval = 2 * time.Second

// TailRunLogs returns an iterator that streams log lines from an ACR task run.
// It polls the log blob with range requests and checks run status to detect completion.
// The iterator yields one line at a time. It stops when the run reaches a terminal
// state and all available log content has been read.
func (a *ACR) TailRunLogs(ctx context.Context, runID string) (iter.Seq2[string, error], error) {
	logURL, err := a.GetRunLogURL(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get log URL: %w", err)
	}

	return func(yield func(string, error) bool) {
		var offset int64

		for {
			newOffset, err := readLogChunk(ctx, logURL, offset, yield)
			offset = newOffset
			if err != nil {
				return // yield returned false, stop iteration
			}

			// Check if the run is done
			status, err := a.GetRunStatus(ctx, runID)
			if err != nil {
				yield("", fmt.Errorf("failed to get run status: %w", err))
				return
			}
			if status.IsTerminal() {
				// Read any remaining log content
				readLogChunk(ctx, logURL, offset, yield)
				if !status.IsSuccess() {
					msg := string(status.Status)
					if status.ErrorMessage != "" {
						msg += ": " + status.ErrorMessage
					}
					yield("", fmt.Errorf("build %s: %s", runID, msg))
				}
				return
			}

			select {
			case <-ctx.Done():
				yield("", ctx.Err())
				return
			case <-time.After(logPollInterval):
			}
		}
	}, nil
}

// readLogChunk fetches new log content starting at offset and yields each line.
// Returns the new offset and a non-nil error if yield returned false.
// Incomplete lines (no trailing newline) are held back until the next poll.
func readLogChunk(ctx context.Context, logURL string, offset int64, yield func(string, error) bool) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, logURL, nil)
	if err != nil {
		if !yield("", err) {
			return offset, errStopped
		}
		return offset, nil
	}
	req.Header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if !yield("", fmt.Errorf("failed to fetch logs: %w", err)) {
			return offset, errStopped
		}
		return offset, nil
	}
	defer resp.Body.Close()

	// 206 Partial Content = new data; 416 Range Not Satisfiable = no new data yet
	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		return offset, nil
	}
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		if !yield("", fmt.Errorf("unexpected log response status: %s", resp.Status)) {
			return offset, errStopped
		}
		return offset, nil
	}

	// Read all new content and split into lines. Only yield complete lines
	// (terminated by \n). Hold back the last incomplete line by not advancing
	// offset past it — it will be re-fetched on the next poll.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if !yield("", fmt.Errorf("failed to read log body: %w", err)) {
			return offset, errStopped
		}
		return offset, nil
	}

	text := string(body)
	if len(text) == 0 {
		return offset, nil
	}
	lines := strings.Split(text, "\n")

	// If the last byte is not a newline, the last line is incomplete — hold it back
	complete := lines
	if text[len(text)-1] != '\n' {
		complete = lines[:len(lines)-1]
	}

	for _, line := range complete {
		offset += int64(len(line) + 1) // +1 for the newline
		if !yield(line, nil) {
			return offset, errStopped
		}
	}

	return offset, nil
}

// errStopped is a sentinel error indicating the consumer stopped iteration.
var errStopped = errors.New("iteration stopped")
