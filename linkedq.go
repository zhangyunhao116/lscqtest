package lscq

import "sync"

type linkedqueue struct {
	head *linkedqueueNode
	tail *linkedqueueNode
	mu   sync.Mutex
}

type linkedqueueNode struct {
	value uint64
	next  *linkedqueueNode
}

func newLQ() *linkedqueue {
	node := new(linkedqueueNode)
	return &linkedqueue{head: node, tail: node}
}

func (q *linkedqueue) Enqueue(value uint64) bool {
	q.mu.Lock()
	q.tail.next = &linkedqueueNode{value: value}
	q.tail = q.tail.next
	q.mu.Unlock()
	return true
}

func (q *linkedqueue) Dequeue() (uint64, bool) {
	q.mu.Lock()
	if q.head.next == nil {
		q.mu.Unlock()
		return 0, false
	} else {
		value := q.head.next.value
		q.head = q.head.next
		q.mu.Unlock()
		return value, true
	}
}
