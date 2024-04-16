package main

type CircularList[T any] struct {
	values   []T
	position int
}

func NewCircularList[T any](vs []T) *CircularList[T] {
	return &CircularList[T]{
		values:   vs,
		position: 0,
	}
}

func (cl *CircularList[T]) Replace(newValues []T) {
	cl.values = newValues
	cl.position = 0
}

func (cl *CircularList[T]) Clear() {
	cl.values = nil
	cl.position = 0
}

func (cl *CircularList[T]) Current() T {
	if cl.position < len(cl.values) {
		return cl.values[cl.position]
	}

	var value T
	return value
}

func (cl *CircularList[T]) PeekNext() T {
	p := cl.nextPosition()
	if p < len(cl.values) {
		return cl.values[p]
	}

	var value T
	return value
}

func (cl *CircularList[T]) Advance() {
	cl.position = cl.nextPosition()
}

func (cl *CircularList[T]) Length() int {
	return len(cl.values)
}

func (cl *CircularList[T]) nextPosition() int {
	p := cl.position + 1
	if p >= len(cl.values) {
		p = 0
	}
	return p
}
