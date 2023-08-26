package circularbuffer

import "sync"

type CircularBuffer[T any] struct {
	values   []T
	position int
	full     bool
	mu       sync.Mutex
}

func New[T any](size int) *CircularBuffer[T] {
	v := make([]T, size)

	return &CircularBuffer[T]{
		values:   v,
		position: 0,
	}
}

func (cb *CircularBuffer[T]) Push(element T) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.values[cb.position] = element
	cb.position++

	if cb.position >= len(cb.values) {
		cb.position = 0
		cb.full = true
	}
}

// Each iterates over all elements in the buffer in the order they were inserted
func (cb *CircularBuffer[T]) Each(fn func(T)) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	i := 0
	if cb.full {
		i = cb.position
	}

	for n := 0; n < len(cb.values); n++ {
		fn(cb.values[i])

		i++
		if i >= len(cb.values) {
			i = 0
		}
		if i == cb.position {
			return
		}
	}
}
