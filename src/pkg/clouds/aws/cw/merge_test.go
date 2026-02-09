package cw

import (
	"errors"
	"iter"
	"testing"

	"github.com/aws/smithy-go/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func logEvents(timestamps ...int64) iter.Seq2[LogEvent, error] {
	return func(yield func(LogEvent, error) bool) {
		for _, ts := range timestamps {
			if !yield(LogEvent{Timestamp: ptr.Int64(ts)}, nil) {
				return
			}
		}
	}
}

func collect(seq iter.Seq2[LogEvent, error]) ([]int64, error) {
	var timestamps []int64
	for evt, err := range seq {
		if err != nil {
			return timestamps, err
		}
		timestamps = append(timestamps, *evt.Timestamp)
	}
	return timestamps, nil
}

func TestMergeLogEvents(t *testing.T) {
	tests := []struct {
		name     string
		left     iter.Seq2[LogEvent, error]
		right    iter.Seq2[LogEvent, error]
		expected []int64
	}{
		{
			name:     "both empty",
			left:     logEvents(),
			right:    logEvents(),
			expected: nil,
		},
		{
			name:     "left nil",
			left:     nil,
			right:    logEvents(1, 3, 5),
			expected: []int64{1, 3, 5},
		},
		{
			name:     "right nil",
			left:     logEvents(2, 4, 6),
			right:    nil,
			expected: []int64{2, 4, 6},
		},
		{
			name:     "both nil",
			left:     nil,
			right:    nil,
			expected: nil,
		},
		{
			name:     "interleaved",
			left:     logEvents(1, 3, 5),
			right:    logEvents(2, 4, 6),
			expected: []int64{1, 2, 3, 4, 5, 6},
		},
		{
			name:     "left before right",
			left:     logEvents(1, 2, 3),
			right:    logEvents(4, 5, 6),
			expected: []int64{1, 2, 3, 4, 5, 6},
		},
		{
			name:     "right before left",
			left:     logEvents(4, 5, 6),
			right:    logEvents(1, 2, 3),
			expected: []int64{1, 2, 3, 4, 5, 6},
		},
		{
			name:     "equal timestamps",
			left:     logEvents(1, 2, 3),
			right:    logEvents(1, 2, 3),
			expected: []int64{1, 1, 2, 2, 3, 3},
		},
		{
			name:     "left empty",
			left:     logEvents(),
			right:    logEvents(1, 2, 3),
			expected: []int64{1, 2, 3},
		},
		{
			name:     "right empty",
			left:     logEvents(1, 2, 3),
			right:    logEvents(),
			expected: []int64{1, 2, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged := MergeLogEvents(tt.left, tt.right)
			if merged == nil {
				assert.Nil(t, tt.expected)
				return
			}
			got, err := collect(merged)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestMergeLogEvents_Error(t *testing.T) {
	testErr := errors.New("test error")
	errSeq := func(yield func(LogEvent, error) bool) {
		yield(LogEvent{}, testErr)
	}

	t.Run("left error", func(t *testing.T) {
		merged := MergeLogEvents(errSeq, logEvents(1, 2))
		_, err := collect(merged)
		assert.Equal(t, testErr, err)
	})

	t.Run("right error", func(t *testing.T) {
		merged := MergeLogEvents(logEvents(1, 2), errSeq)
		_, err := collect(merged)
		assert.Equal(t, testErr, err)
	})
}

func TestMergeLogEvents_EarlyStop(t *testing.T) {
	merged := MergeLogEvents(logEvents(1, 3, 5, 7, 9), logEvents(2, 4, 6, 8, 10))
	got := TakeFirstN(merged, 4)
	ts, err := collect(got)
	require.NoError(t, err)
	assert.Equal(t, []int64{1, 2, 3, 4}, ts)
}

func TestTakeFirstN(t *testing.T) {
	tests := []struct {
		name     string
		input    []int64
		n        int
		expected []int64
	}{
		{"take 3 of 5", []int64{1, 2, 3, 4, 5}, 3, []int64{1, 2, 3}},
		{"take 5 of 3", []int64{1, 2, 3}, 5, []int64{1, 2, 3}},
		{"take 0", []int64{1, 2, 3}, 0, []int64{1, 2, 3}},
		{"take negative", []int64{1, 2, 3}, -1, []int64{1, 2, 3}},
		{"empty input", nil, 3, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := collect(TakeFirstN(logEvents(tt.input...), tt.n))
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestTakeLastN(t *testing.T) {
	tests := []struct {
		name     string
		input    []int64
		n        int
		expected []int64
	}{
		{"last 3 of 5", []int64{1, 2, 3, 4, 5}, 3, []int64{3, 4, 5}},
		{"last 5 of 3", []int64{1, 2, 3}, 5, []int64{1, 2, 3}},
		{"last 0", []int64{1, 2, 3}, 0, []int64{1, 2, 3}},
		{"last negative", []int64{1, 2, 3}, -1, []int64{1, 2, 3}},
		{"empty input", nil, 3, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := collect(TakeLastN(logEvents(tt.input...), tt.n))
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestTakeLastN_Error(t *testing.T) {
	testErr := errors.New("test error")
	seq := func(yield func(LogEvent, error) bool) {
		if yield(LogEvent{Timestamp: ptr.Int64(1)}, nil) {
			yield(LogEvent{}, testErr)
		}
	}
	_, err := collect(TakeLastN(seq, 5))
	assert.Equal(t, testErr, err)
}
