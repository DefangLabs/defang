package cw

import "iter"

// MergeLogEvents merge-sorts two ascending iterators by Timestamp.
// Uses iter.Pull2 internally for two-pointer merge.
func MergeLogEvents(left, right iter.Seq2[LogEvent, error]) iter.Seq2[LogEvent, error] {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	return func(yield func(LogEvent, error) bool) {
		nextL, stopL := iter.Pull2(left)
		defer stopL()
		nextR, stopR := iter.Pull2(right)
		defer stopR()

		lVal, lErr, lOk := nextL()
		rVal, rErr, rOk := nextR()

		for lOk && rOk {
			if lErr != nil {
				yield(lVal, lErr)
				return
			}
			if rErr != nil {
				yield(rVal, rErr)
				return
			}
			if *lVal.Timestamp <= *rVal.Timestamp {
				if !yield(lVal, nil) {
					return
				}
				lVal, lErr, lOk = nextL()
			} else {
				if !yield(rVal, nil) {
					return
				}
				rVal, rErr, rOk = nextR()
			}
		}

		for lOk {
			if !yield(lVal, lErr) {
				return
			}
			if lErr != nil {
				return
			}
			lVal, lErr, lOk = nextL()
		}
		for rOk {
			if !yield(rVal, rErr) {
				return
			}
			if rErr != nil {
				return
			}
			rVal, rErr, rOk = nextR()
		}
	}
}

// TakeFirstN yields at most n items from the iterator, then stops.
func TakeFirstN(seq iter.Seq2[LogEvent, error], n int) iter.Seq2[LogEvent, error] {
	if n <= 0 {
		return seq
	}
	return func(yield func(LogEvent, error) bool) {
		count := 0
		for evt, err := range seq {
			if !yield(evt, err) {
				return
			}
			if err != nil {
				return
			}
			count++
			if count >= n {
				return
			}
		}
	}
}

// TakeLastN buffers the entire input, then yields the last n items.
func TakeLastN(seq iter.Seq2[LogEvent, error], n int) iter.Seq2[LogEvent, error] {
	if n <= 0 {
		return seq
	}
	return func(yield func(LogEvent, error) bool) {
		var buffer []LogEvent
		for evt, err := range seq {
			if err != nil {
				yield(evt, err)
				return
			}
			buffer = append(buffer, evt)
			if len(buffer) > n {
				buffer = buffer[1:]
			}
		}
		for _, evt := range buffer {
			if !yield(evt, nil) {
				return
			}
		}
	}
}
