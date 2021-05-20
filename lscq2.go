package lscq

import (
	"sync/atomic"
	"unsafe"
)

const (
	maskIsSafe = 1 << 63
	maskIndex  = ((1 << 19) - 1) << 44
	maskCycle  = (1 << 44) - 1
	maxIndex   = (1 << 19) - 1
)

type lscq2 struct {
	head *scq2
	_    [cacheLineSize - unsafe.Sizeof(new(uintptr))]byte
	tail *scq2
}

func newLSCQ2() *lscq2 {
	q := newSCQ2(scqsize)
	return &lscq2{head: q, tail: q}
}

func (q *lscq2) Enqueue(data uint64) bool {
	for {
		cq := (*scq2)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head))))
		nex := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&cq.next)))
		if nex != nil {
			// Help move cq.next into tail.
			atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail)), (unsafe.Pointer(cq)), nex)
			continue
		}
	}
}

type scq2 struct {
	aq   *innerSCQ2
	fq   *innerSCQ2
	next *scq2
	data []uint64
}

func newSCQ2(size int) *scq2 {
	return &scq2{
		aq:   newInnerSCQ2Empty(size),
		fq:   newInnerSCQ2Full(size),
		data: make([]uint64, size),
	}
}

func (q *scq2) Enqueue(data uint64) bool {
	idx, ok := q.fq.Dequeue()
	if !ok {
		return false
	}
	q.data[idx] = data
	q.aq.Enqueue(idx)
	return true
}

func (q *scq2) Dequeue() (data uint64, ok bool) {
	idx, ok := q.aq.Dequeue()
	if !ok {
		return
	}
	data = q.data[idx]
	q.fq.Enqueue(idx)
	return data, true
}

type innerSCQ2 struct {
	head      uint64
	_         [cacheLineSize - unsafe.Sizeof(new(uint64))]byte
	tail      uint64 // 1-bit finalize + 63-bit tail
	_         [cacheLineSize - unsafe.Sizeof(new(uint64))]byte
	threshold int64
	_         [cacheLineSize - unsafe.Sizeof(new(uint64))]byte
	next      *innerSCQ2
	n         int
	ring      []uint64 // entry: 1-bit isSafe + 19-bit index + 44-bit cycle
}

func newInnerSCQ2Empty(size int) *innerSCQ2 {
	ring := make([]uint64, size*2)
	for i := range ring {
		ring[i] = newInnerSCQEntry(true, maxIndex, 0) // 1<<63 + math.MaxUint64&maskIndex
	}
	return &innerSCQ2{
		head:      uint64(2 * size),
		tail:      uint64(2 * size),
		threshold: -1,
		n:         size,
		ring:      ring,
	}
}

func newInnerSCQ2Full(size int) *innerSCQ2 {
	q := newInnerSCQ2Empty(size)
	// for i := 0; i < size; i++ {
	// 	q.Enqueue(uint64(i))
	// }
	q.head = 0
	for i := 0; i < size; i++ {
		q.ring[cacheRemap(i, size*2)] = uint64(1<<63) + uint64(i<<19)
	}
	return q
}

func atomicIncrUint64(addr *uint64) uint64 {
	return atomic.AddUint64(addr, 1) - 1 // return previous value
}

func loadInnerSCQEntry(v uint64) (bool, uint64, uint64) {
	isSafe := v&maskIsSafe == maskIsSafe
	index := (v & maskIndex) >> 44
	cycle := v & maskCycle
	return isSafe, index, cycle
}

func newInnerSCQEntry(isSafe bool, index, cycle uint64) uint64 {
	var res uint64
	if isSafe {
		res += maskIsSafe
	}
	res += (index << 44)
	res += cycle
	return res
}

func (q *innerSCQ2) Enqueue(index uint64) bool {
	for {
		T := atomicIncrUint64(&q.tail)
		entAddr := &q.ring[cacheRemap(int(T&uint64(2*q.n-1)), cap(q.ring))]
		cycleT := T / uint64(2*q.n)
	eqretry:
		ent := atomic.LoadUint64(entAddr)
		isSafe, indexE, cycleE := loadInnerSCQEntry(ent)
		if cycleE < cycleT && indexE == maxIndex && (isSafe || atomic.LoadUint64(&q.head) <= T) {
			newEnt := newInnerSCQEntry(true, index, cycleT)
			if !atomic.CompareAndSwapUint64(entAddr, ent, newEnt) {
				goto eqretry
			}
			if atomic.LoadInt64(&q.threshold) != int64(3*q.n)-1 {
				atomic.StoreInt64(&q.threshold, int64(3*q.n)-1)
			}
			return true
		}
	}
}

func (q *innerSCQ2) Dequeue() (value uint64, ok bool) {
	if atomic.LoadInt64(&q.threshold) < 0 {
		return
	}
	for {
		H := atomicIncrUint64(&q.head)
		entAddr := &q.ring[cacheRemap(int(H&uint64(2*q.n-1)), cap(q.ring))]
		cycleH := H / uint64(2*q.n)
	dqretry:
		ent := atomic.LoadUint64(entAddr)
		isSafe, indexE, cycleE := loadInnerSCQEntry(ent)
		if cycleE == cycleH {
			atomic.StoreUint64(entAddr, newInnerSCQEntry(isSafe, maxIndex, cycleE))
			return indexE, true
		}
		newEnt := newInnerSCQEntry(false, indexE, cycleE)
		if indexE == maxIndex {
			newEnt = newInnerSCQEntry(isSafe, maxIndex, cycleH)
		}
		if cycleE < cycleH {
			if !atomic.CompareAndSwapUint64(entAddr, ent, newEnt) {
				goto dqretry
			}
		}
		T := atomic.LoadUint64(&q.tail)
		if T <= H+1 {
			q.catchup(T, H+1)
			atomic.AddInt64(&q.threshold, -1)
			return
		}
		if atomic.AddInt64(&q.threshold, -1)+1 <= 0 {
			return
		}
	}
}

func (q *innerSCQ2) catchup(tail, head uint64) {
	for !atomic.CompareAndSwapUint64(&q.tail, tail, head) {
		head := atomic.LoadUint64(&q.head)
		tail := atomic.LoadUint64(&q.tail)
		if tail >= head {
			break
		}
	}
}
