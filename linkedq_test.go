package lscq

import (
	"sync"
	"testing"

	"github.com/zhangyunhao116/fastrand"
	"github.com/zhangyunhao116/skipset"
)

func TestUnboundedLinkedQueue(t *testing.T) {
	// MPMC correctness.
	q := newLQ()
	var wg sync.WaitGroup
	s1 := skipset.NewUint64()
	s2 := skipset.NewUint64()
	for i := 0; i < 100000; i++ {
		wg.Add(1)
		go func() {
			if fastrand.Uint32n(2) == 0 {
				r := fastrand.Uint64()
				if !s1.Add(r) || !q.Enqueue(r) {
					panic("invalid")
				}
			} else {
				val, ok := q.Dequeue()
				if ok {
					s2.Add(uint64(val))
				}
			}
			wg.Done()
		}()
	}
	wg.Wait()

	for {
		val, ok := q.Dequeue()
		if !ok {
			break
		}
		s2.Add(uint64(val))
	}

	s1.Range(func(value uint64) bool {
		if !s2.Contains(value) {
			t.Fatal(value)
		}
		return true
	})

	if s1.Len() != s2.Len() {
		t.Fatal("invalid")
	}
}
