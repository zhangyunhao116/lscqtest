package lscq

import (
	"unsafe"

	"golang.org/x/sys/cpu"
)

const (
	scqsize       = 1 << 12
	cacheLineSize = unsafe.Sizeof(cpu.CacheLinePad{})
)

func uint64Get63(value uint64) uint64 {
	return value & ((1 << 63) - 1)
}

func uint64Get1(value uint64) bool {
	return (value & (1 << 63)) == (1 << 63)
}

func uint64GetAll(value uint64) (bool, uint64) {
	return (value & (1 << 63)) == (1 << 63), value & ((1 << 63) - 1)
}

func loadSCQFlags(flags uint64) (isSafe bool, isEmpty bool, cycle uint64) {
	isSafe = (flags & (1 << 63)) == (1 << 63)
	isEmpty = (flags & (1 << 62)) == (1 << 62)
	cycle = flags & ((1 << 62) - 1)
	return isSafe, isEmpty, cycle
}

func newSCQFlags(isSafe bool, isEmpty bool, cycle uint64) uint64 {
	v := cycle & ((1 << 62) - 1)
	if isSafe {
		v += 1 << 63
	}
	if isEmpty {
		v += 1 << 62
	}
	return v
}
