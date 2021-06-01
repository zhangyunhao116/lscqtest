package lscq

type channelQ chan uint64

func newChannelQ(size int) channelQ {
	return (channelQ)(make(chan uint64, size))
}

func (q channelQ) Enqueue(data uint64) bool {
	q <- data
	return true
}

func (q channelQ) Dequeue() (uint64, bool) {
	data := <-q
	return data, true
}
