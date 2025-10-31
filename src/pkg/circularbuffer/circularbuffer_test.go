package circularbuffer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCircularBuffer(t *testing.T) {
	buffer := NewCircularBuffer[int](3)

	assert.Equal(t, []int{}, buffer.Get())
	buffer.Add(1)
	assert.Equal(t, []int{1}, buffer.Get())
	buffer.Add(2)
	assert.Equal(t, []int{1, 2}, buffer.Get())
	buffer.Add(3)
	assert.Equal(t, []int{1, 2, 3}, buffer.Get())
	buffer.Add(4)
	assert.Equal(t, []int{2, 3, 4}, buffer.Get())
	buffer.Add(5)
	assert.Equal(t, []int{3, 4, 5}, buffer.Get())
}
