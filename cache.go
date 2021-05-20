package lscq

func cacheRemap4096(index int) int {
	cacheLineNum := index % int(cacheLineSize)
	idx := index / int(cacheLineSize)
	return cacheLineNum*int(cacheLineSize) + idx
}
