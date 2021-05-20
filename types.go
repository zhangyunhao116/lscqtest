// Code generated by go run types_gen.go; DO NOT EDIT.
package lscq

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

var uint64SCQpool = sync.Pool{
	New: func() interface{} {
		return newUint64SCQ(scqsize)
	},
}

type Uint64LSCQ struct {
	head *uint64SCQ
	_    [cacheLineSize - unsafe.Sizeof(new(uintptr))]byte
	tail *uint64SCQ
}

func NewUint64() *Uint64LSCQ {
	q := newUint64SCQ(scqsize)
	return &Uint64LSCQ{head: q, tail: q}
}

func (q *Uint64LSCQ) Dequeue() (data uint64, ok bool) {
	for {
		cq := (*uint64SCQ)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head))))
		data, ok = cq.Dequeue()
		if ok {
			return
		}
		// cq does not have enough entries.
		nex := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&cq.next)))
		if nex == nil {
			// We don't have next SCQ.
			return
		}
		// cq.next is not empty, subsequent entry will be insert into cq.next instead of cq.
		// So if cq is empty, we can move it into ncqpool.
		atomic.StoreInt64(&cq.threshold, int64(scqsize*2)-1)
		data, ok = cq.Dequeue()
		if ok {
			return
		}
		if atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head)), (unsafe.Pointer(cq)), nex) {
			// We can't ensure no other goroutines will access cq.
			// This queue can still be previous dequeue's cq.
			// scqpool.Put(cq)
			cq = nil
		}
	}
}

func (q *Uint64LSCQ) Enqueue(data uint64) bool {
	for {
		cq := (*uint64SCQ)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail))))
		nex := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&cq.next)))
		if nex != nil {
			// Help move cq.next into tail.
			atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail)), (unsafe.Pointer(cq)), nex)
			continue
		}
		if cq.Enqueue(data) {
			return true
		}
		// Concurrent cq is full.
		atomicTestAndSetFirstBit(&cq.tail)      // close cq, subsequent enqueue will fail
		ncq := uint64SCQpool.Get().(*uint64SCQ) // create a new queue
		// ncq.reset()
		ncq.Enqueue(data)
		// Try Add this queue into cq.next.
		if atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&cq.next)), nil, unsafe.Pointer(ncq)) {
			// Success.
			// Try move cq.next into tail (we don't need to recheck since other enqueuer will help).
			atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail)), unsafe.Pointer(cq), unsafe.Pointer(ncq))
			return true
		}
		ncq.Dequeue()
		// CAS failed, put this new SCQ into scqpool.
		// No other goroutines will access this queue.
		uint64SCQpool.Put(ncq)
	}
}

type uint64SCQ struct {
	head         uint64
	_            [cacheLineSize - unsafe.Sizeof(new(uint64))]byte
	tail         uint64 // 1-bit finalize + 63-bit tail
	_            [cacheLineSize - unsafe.Sizeof(new(uint64))]byte
	threshold    int64
	_            [cacheLineSize - unsafe.Sizeof(new(uint64))]byte
	next         *uint64SCQ
	ring         []scqNodeUint64
	_            [cacheLineSize - unsafe.Sizeof(new(uint64))*3]byte
	enqueueLimit int64
	_            [cacheLineSize - unsafe.Sizeof(new(uint64))]byte
	dequeueLimit int64
}

type scqNodeUint64 struct {
	flags uint64 // isSafe 1-bit + isEmpty 1-bit + cycle 62-bit
	data  uint64
}

func newUint64SCQ(n int) *uint64SCQ {
	if n < 0 || n%2 != 0 {
		panic("n must bigger than zero AND n % 2 == 0")
	}
	ring := make([]scqNodeUint64, n)
	for i := range ring {
		ring[i].flags = 1<<63 + 1<<62 // newSCQFlags(true, true, 0)
	}
	return &uint64SCQ{
		head:         uint64(n),
		tail:         uint64(n),
		threshold:    -1,
		ring:         ring,
		enqueueLimit: int64(n),
		dequeueLimit: int64(n),
	}
}

func (q *uint64SCQ) reset() {
	q.tail = scqsize
	q.head = scqsize
	q.threshold = -1
	q.next = nil
	for i := range q.ring {
		q.ring[i].flags = 1<<63 + 1<<62
	}
}

func (q *uint64SCQ) Enqueue(data uint64) bool {
	// If TAIL >= HEAD + 2n, means this queue is full.
	qhead := atomic.LoadUint64(&q.head)
	if uint64Get63(atomic.LoadUint64(&q.tail)) >= qhead+scqsize {
		return false
	}
	// Enqueue limit
	for {
		if atomic.AddInt64(&q.enqueueLimit, -1) < 0 {
			panic("!")
			atomic.AddInt64(&q.enqueueLimit, 1)
			continue
		} else {
			defer atomic.AddInt64(&q.enqueueLimit, 1)
			break
		}
	}
	for {
		// Increment the TAIL, try to occupy an entry.
		tailvalue := atomic.AddUint64(&q.tail, 1)
		tailvalue -= 1 // we need previous value
		T := uint64Get63(tailvalue)
		if uint64Get1(tailvalue) { // the queue is closed
			return false
		}
		entAddr := &q.ring[cacheRemap4096(int(T&uint64(scqsize-1)))]
		cycleT := T / scqsize
	eqretry:
		ent := loadSCQNodeUint64(unsafe.Pointer(entAddr))
		isSafe, isEmpty, cycleEnt := loadSCQFlags(ent.flags)
		if cycleEnt < cycleT && isEmpty && (isSafe || atomic.LoadUint64(&q.head) <= T) {
			// We can use this entry for adding new data if
			// 1. cycleEnt < cycleT
			// 2. It is empty
			// 3. It is safe or tail >= head (There is enough space for this data)
			newEnt := scqNodeUint64{flags: newSCQFlags(true, false, cycleT), data: data}
			// Save input data into this entry.
			if !compareAndSwapSCQNodeUint64(entAddr, ent, newEnt) {
				// Failed, do next retry.
				goto eqretry
			}
			// Success.
			if atomic.LoadInt64(&q.threshold) != (int64(scqsize)*2)-1 {
				atomic.StoreInt64(&q.threshold, (int64(scqsize)*2)-1)
			}
			return true
		}
		// Add a full queue check in the loop(CAS2).
		if T+1 >= qhead+scqsize {
			qhead := atomic.LoadUint64(&q.head)
			if T+1 >= qhead+scqsize {
				return false
			}
		}
	}
}

func (q *uint64SCQ) Dequeue() (data uint64, ok bool) {
	if atomic.LoadInt64(&q.threshold) < 0 {
		// Empty queue.
		return
	}
	// Dequeue limit
	for {
		if atomic.AddInt64(&q.dequeueLimit, -1) < 0 {
			panic("!")
			atomic.AddInt64(&q.dequeueLimit, 1)
			continue
		} else {
			defer atomic.AddInt64(&q.dequeueLimit, 1)
			break
		}
	}
	for {
		// Decrement HEAD, try to release an entry.
		H := atomic.AddUint64(&q.head, 1)
		H -= 1 // we need previous value
		entAddr := &q.ring[cacheRemap4096(int(H&uint64(scqsize-1)))]
		cycleH := H / scqsize
	dqretry:
		ent := loadSCQNodeUint64(unsafe.Pointer(entAddr))
		isSafe, isEmpty, cycleEnt := loadSCQFlags(ent.flags)
		if cycleEnt == cycleH { // same cycle, return this entry directly
			atomicTestAndSetSecondBit(&entAddr.flags)
			// Special case, if the data type is `unsafe.Pointer` we need reset it.

			data, ok = ent.data, true
			return
		}
		// Try to mark this node unsafe.
		newEnt := scqNodeUint64{flags: newSCQFlags(false, false, cycleEnt), data: ent.data}
		if isEmpty {
			newEnt = scqNodeUint64{flags: newSCQFlags(isSafe, true, cycleH)}
		}
		if cycleEnt < cycleH {
			if !compareAndSwapSCQNodeUint64(entAddr, ent, newEnt) {
				goto dqretry
			}
		}
		// check if the queue is empty
		tailvalue := atomic.LoadUint64(&q.tail)
		T := uint64Get63(tailvalue)
		if T <= H+1 {
			// The queue is empty.
			q.catchup(tailvalue, H+1)
			atomic.AddInt64(&q.threshold, -1)
			return
		}
		if atomic.AddInt64(&q.threshold, -1)+1 <= 0 {
			return
		}
	}
}

func (q *uint64SCQ) catchup(tailvalue, head uint64) {
	for {
		if atomic.CompareAndSwapUint64(&q.tail, tailvalue, head) {
			break
		}
		head = atomic.LoadUint64(&q.head)
		tailvalue = atomic.LoadUint64(&q.tail)
		tail := uint64Get63(tailvalue)
		if tail >= head {
			break
		}
	}
}
