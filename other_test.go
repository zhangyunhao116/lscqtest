package lscq

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/zhangyunhao116/fastrand"
	"github.com/zhangyunhao116/skipset"
)

func TestOther(t *testing.T) {
	return
	var wg sync.WaitGroup
	var a int64
	s := skipset.NewInt64Desc()
	q := NewUint64()
	for i := 0; i < 100000000; i++ {
		wg.Add(1)
		go func() {
			atomic.AddInt64(&a, 1)
			q.Enqueue(fastrand.Uint64())
			s.Add(atomic.AddInt64(&a, -1))
			wg.Done()
		}()
	}
	wg.Wait()
	s.Range(func(value int64) bool {
		println(value)
		return false
	})
}
