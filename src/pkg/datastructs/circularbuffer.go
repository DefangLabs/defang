package datastructs

var DefaultBufferSize = 10

type CircularBuffer[T any] struct {
	size    int
	entries int
	index   int
	data    []T
}

func (c *CircularBuffer[T]) Add(item T) {
	c.entries++
	c.data[c.index] = item
	c.index = (c.index + 1) % c.size
}

func (c *CircularBuffer[T]) Get() []T {
	maxItems := min(c.entries, c.size)
	items := make([]T, 0, maxItems)
	startIdx := c.index

	// the c.index points to the next write position (ie. oldest entry) if the buffer is full,
	// otherwise if the buffer is not full then c.index does not point to the older entry so we
	// need to start from index 0
	if c.entries < c.size {
		startIdx = 0
	}

	// Collect items in chronological order
	for i := range maxItems {
		idx := (startIdx + i) % c.size
		items = append(items, c.data[idx])
	}
	return items
}

func NewCircularBuffer[T any](bufferSize int) CircularBuffer[T] {
	if bufferSize <= 0 {
		panic("failed to created a circular buffer: cannot have zero elements")
	}
	return CircularBuffer[T]{
		size:    bufferSize,
		entries: 0,
		index:   0,
		data:    make([]T, bufferSize),
	}
}
