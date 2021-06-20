package lscq

func cacheRemap16Byte(index uint64) uint64 {
	const cacheLineSize = cacheLineSize / 2
	rawIndex := index & uint64(scqsize-1)
	cacheLineNum := (rawIndex) % (scqsize / uint64(cacheLineSize))
	cacheLineIdx := rawIndex / (scqsize / uint64(cacheLineSize))
	return cacheLineNum*uint64(cacheLineSize) + cacheLineIdx
}
