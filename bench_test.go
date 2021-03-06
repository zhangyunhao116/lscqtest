package lscq

import (
	"sync/atomic"
	"testing"

	"github.com/zhangyunhao116/fastrand"
)

type uint64queue interface {
	Enqueue(uint64) bool
	Dequeue() (uint64, bool)
}

type benchTask struct {
	name string
	New  func() uint64queue
}

type faa int64

func (data *faa) Enqueue(_ uint64) bool {
	atomic.AddInt64((*int64)(data), 1)
	return true
}

func (data *faa) Dequeue() (uint64, bool) {
	atomic.AddInt64((*int64)(data), -1)
	return 0, false
}

type mixedQueue struct {
	scq *uint64SCQ
	lq  *msqv1
}

func newMixedQueue() *mixedQueue {
	scq := newUint64SCQ(scqsize)
	lq := newMSQv1()
	return &mixedQueue{scq: scq, lq: lq}
}

func (q *mixedQueue) Enqueue(data uint64) bool {
	if q.scq.Enqueue(data) {
		return true
	}
	return q.lq.Enqueue(data)
}

func (q *mixedQueue) Dequeue() (uint64, bool) {
	data, ok := q.lq.Dequeue()
	if ok {
		return data, ok
	}
	return q.scq.Dequeue()
}

func BenchmarkDefault(b *testing.B) {
	all := []benchTask{{
		name: "LSCQ", New: func() uint64queue {
			return NewUint64()
		}}}
	// all = append(all, benchTask{
	// 	name: "linkedQ",
	// 	New: func() uint64queue {
	// 		return newLQ()
	// 	},
	// })
	// all = append(all, benchTask{
	// 	name: "msqueue",
	// 	New: func() uint64queue {
	// 		return newMSQv1()
	// 	},
	// })
	// all = append(all, benchTask{
	// 	name: "FAA",
	// 	New: func() uint64queue {
	// 		return new(faa)
	// 	},
	// })
	// all = append(all, benchTask{
	// 	name: "channel",
	// 	New: func() uint64queue {
	// 		return newChannelQ(scqsize)
	// 	},
	// })
	// all = append(all, benchTask{
	// 	name: "LCRQ",
	// 	New: func() uint64queue {
	// 		return newLCRQ()
	// 	},
	// })
	// all = append(all, benchTask{
	// 	name: "Pool",
	// 	New: func() uint64queue {
	// 		return NewPoolQueueUint64()
	// 	},
	// })
	benchEnqueueOnly(b, all)
	benchDequeueOnlyEmpty(b, all)
	benchPair(b, all)
	bench50Enqueue50Dequeue(b, all)
	bench30Enqueue70Dequeue(b, all)
	bench70Enqueue30Dequeue(b, all)
}

func reportalloc(b *testing.B) {
	// b.SetBytes(8)
	// b.ReportAllocs()
}

func benchPair(b *testing.B, benchTasks []benchTask) {
	for _, v := range benchTasks {
		b.Run("Pair/"+v.name, func(b *testing.B) {
			q := v.New()
			reportalloc(b)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					q.Enqueue(uint64(fastrand.Uint32()))
					q.Dequeue()
				}
			})
		})
	}
}

func bench50Enqueue50Dequeue(b *testing.B, benchTasks []benchTask) {
	for _, v := range benchTasks {
		b.Run("50Enqueue50Dequeue/"+v.name, func(b *testing.B) {
			q := v.New()
			b.ResetTimer()
			reportalloc(b)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					if fastrand.Uint32n(2) == 0 {
						q.Enqueue(uint64(fastrand.Uint32()))
					} else {
						q.Dequeue()
					}
				}
			})
		})
	}
}

func bench70Enqueue30Dequeue(b *testing.B, benchTasks []benchTask) {
	for _, v := range benchTasks {
		b.Run("70Enqueue30Dequeue/"+v.name, func(b *testing.B) {
			q := v.New()
			reportalloc(b)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					if fastrand.Uint32n(10) > 2 {
						q.Enqueue(uint64(fastrand.Uint32()))
					} else {
						q.Dequeue()
					}
				}
			})
		})
	}
}

func bench30Enqueue70Dequeue(b *testing.B, benchTasks []benchTask) {
	for _, v := range benchTasks {
		b.Run("30Enqueue70Dequeue/"+v.name, func(b *testing.B) {
			q := v.New()
			reportalloc(b)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					if fastrand.Uint32n(10) <= 2 {
						q.Enqueue(uint64(fastrand.Uint32()))
					} else {
						q.Dequeue()
					}
				}
			})
		})
	}
}

func benchEnqueueOnly(b *testing.B, benchTasks []benchTask) {
	for _, v := range benchTasks {
		b.Run("EnqueueOnly/"+v.name, func(b *testing.B) {
			q := v.New()
			reportalloc(b)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					q.Enqueue(uint64(fastrand.Uint32()))
				}
			})
		})
	}
}

func benchDequeueOnlyEmpty(b *testing.B, benchTasks []benchTask) {
	for _, v := range benchTasks {
		b.Run("DequeueOnlyEmpty/"+v.name, func(b *testing.B) {
			q := v.New()
			reportalloc(b)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					q.Dequeue()
				}
			})
		})
	}
}
