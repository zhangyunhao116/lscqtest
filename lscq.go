package lscq

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

var pointerSCQpool = sync.Pool{
	New: func() interface{} {
		return newPointerSCQ(scqsize)
	},
}

type PointerLSCQ struct {
	head *pointerSCQ
	_    [cacheLineSize - unsafe.Sizeof(new(uintptr))]byte
	tail *pointerSCQ
}

func NewPointer() *PointerLSCQ {
	q := newPointerSCQ(scqsize)
	return &PointerLSCQ{head: q, tail: q}
}

func (q *PointerLSCQ) Dequeue() (data unsafe.Pointer, ok bool) {
	for {
		cq := (*pointerSCQ)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head))))
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

func (q *PointerLSCQ) Enqueue(data unsafe.Pointer) bool {
	for {
		cq := (*pointerSCQ)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail))))
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
		atomicTestAndSetFirstBit(&cq.tail)        // close cq, subsequent enqueue will fail
		ncq := pointerSCQpool.Get().(*pointerSCQ) // create a new queue
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
		pointerSCQpool.Put(ncq)
	}
}

type pointerSCQ struct {
	head      uint64
	_         [cacheLineSize - unsafe.Sizeof(new(uint64))]byte
	tail      uint64 // 1-bit finalize + 63-bit tail
	_         [cacheLineSize - unsafe.Sizeof(new(uint64))]byte
	threshold int64
	_         [cacheLineSize - unsafe.Sizeof(new(uint64))]byte
	next      *pointerSCQ
	ring      []scqNodePointer
}

type scqNodePointer struct {
	flags uint64 // isSafe 1-bit + isEmpty 1-bit + cycle 62-bit
	data  unsafe.Pointer
}

func newPointerSCQ(n int) *pointerSCQ {
	if n < 0 || n%2 != 0 {
		panic("n must bigger than zero AND n % 2 == 0")
	}
	ring := make([]scqNodePointer, n)
	for i := range ring {
		ring[i].flags = 1<<63 + 1<<62 // newSCQFlags(true, true, 0)
	}
	return &pointerSCQ{
		head:      uint64(n),
		tail:      uint64(n),
		threshold: -1,
		ring:      ring,
	}
}

func (q *pointerSCQ) reset() {
	q.tail = scqsize
	q.head = scqsize
	q.threshold = -1
	q.next = nil
	for i := range q.ring {
		q.ring[i].flags = 1<<63 + 1<<62
	}
}

func (q *pointerSCQ) Enqueue(data unsafe.Pointer) bool {
	// If TAIL >= HEAD + 2n, means this queue is full.
	qhead := atomic.LoadUint64(&q.head)
	if uint64Get63(atomic.LoadUint64(&q.tail)) >= qhead+scqsize {
		return false
	}

	for {
		// Increment the TAIL, try to occupy an entry.
		tailvalue := atomic.AddUint64(&q.tail, 1)
		tailvalue -= 1 // we need previous value
		T := uint64Get63(tailvalue)
		if uint64Get1(tailvalue) { // the queue is closed
			return false
		}
		entAddr := &q.ring[cacheRemap3(T&uint64(scqsize-1))]
		cycleT := T / scqsize
	eqretry:
		ent := loadSCQNodePointer(unsafe.Pointer(entAddr))
		isSafe, isEmpty, cycleEnt := loadSCQFlags(ent.flags)
		if cycleEnt < cycleT && isEmpty && (isSafe || atomic.LoadUint64(&q.head) <= T) {
			// We can use this entry for adding new data if
			// 1. cycleEnt < cycleT
			// 2. It is empty
			// 3. It is safe or tail >= head (There is enough space for this data)
			newEnt := scqNodePointer{flags: newSCQFlags(true, false, cycleT), data: data}
			// Save input data into this entry.
			if !compareAndSwapSCQNodePointer(entAddr, ent, newEnt) {
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

func (q *pointerSCQ) Dequeue() (data unsafe.Pointer, ok bool) {
	if atomic.LoadInt64(&q.threshold) < 0 {
		// Empty queue.
		return
	}

	for {
		// Decrement HEAD, try to release an entry.
		H := atomic.AddUint64(&q.head, 1)
		H -= 1 // we need previous value
		entAddr := &q.ring[cacheRemap3(H&uint64(scqsize-1))]
		cycleH := H / scqsize
		retrycount := 0
	dqretry:
		ent := loadSCQNodePointer(unsafe.Pointer(entAddr))
		isSafe, isEmpty, cycleEnt := loadSCQFlags(ent.flags)
		if cycleEnt == cycleH { // same cycle, return this entry directly
			atomicTestAndSetSecondBit(&entAddr.flags)
			// Special case, if the data type is `unsafe.Pointer` we need reset it.
			atomic.StorePointer((*unsafe.Pointer)(ent.data), nil)
			return ent.data, true
		}
		if retrycount <= 3 {
			retrycount++
			goto dqretry
		}
		// Try to mark this node unsafe.
		var newEnt scqNodePointer
		if isEmpty {
			newEnt = scqNodePointer{flags: newSCQFlags(isSafe, true, cycleH)}
		} else {
			newEnt = scqNodePointer{flags: newSCQFlags(false, false, cycleEnt), data: ent.data}
		}
		if cycleEnt < cycleH {
			if !compareAndSwapSCQNodePointer(entAddr, ent, newEnt) {
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

func (q *pointerSCQ) catchup(tailvalue, head uint64) {
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
