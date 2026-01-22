package cw

// Inspired by https://dev.to/vinaygo/concurrency-merge-sort-using-channels-and-goroutines-in-golang-35f7
func Mergech[T any](left <-chan T, right <-chan T, c chan<- T, less func(T, T) bool) {
	defer close(c)
	val, ok := <-left
	val2, ok2 := <-right
	for ok && ok2 {
		if less(val, val2) {
			c <- val
			val, ok = <-left
		} else {
			c <- val2
			val2, ok2 = <-right
		}
	}
	for ok {
		c <- val
		val, ok = <-left
	}
	for ok2 {
		c <- val2
		val2, ok2 = <-right
	}
}

func MergeLogEventChan(left, right <-chan LogEvent) <-chan LogEvent {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	out := make(chan LogEvent)
	go Mergech(left, right, out, func(i1, i2 LogEvent) bool {
		return *i1.Timestamp < *i2.Timestamp
	})
	return out
}
