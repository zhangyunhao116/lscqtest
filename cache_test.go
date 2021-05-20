package lscq

import (
	"testing"

	"github.com/zhangyunhao116/fastrand"
	"github.com/zhangyunhao116/skipset"
)

func TestCacheRemap(t *testing.T) {
	ssize := scqsize * 4
	// Full test.
	s := skipset.NewInt()
	for i := 0; i < ssize; i++ {
		s.Add(cacheRemap2(i, ssize))
	}
	var i int
	s.Range(func(value int) bool {
		if i != value {
			t.Fatal()
		}
		i++
		return true
	})

	// Slice test.
	s = skipset.NewInt()
	slice1 := make([]int, ssize)
	for i := 0; i < ssize/2; i++ {
		slice1[cacheRemap2(i, ssize)] = fastrand.Int()
		s.Add(slice1[cacheRemap2(i, ssize)])
	}
	for i := 0; i < ssize/2; i++ {
		if !s.Contains(slice1[cacheRemap2(i, ssize)]) {
			t.Fatal(i)
		}
	}
}
