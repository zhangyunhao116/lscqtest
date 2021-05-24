package lscq

func cacheRemap(index, slicecap int) int {
	// return index
	return cacheRemap2(index, slicecap)
}

func cacheRemap2(index, slicecap int) int {
	if slicecap <= int(cacheLineSize) {
		return index
	}
	cacheLineNum := (index % int(cacheLineSize)) % (slicecap / int(cacheLineSize))
	idx := index / (slicecap / int(cacheLineSize))
	res := cacheLineNum*int(cacheLineSize) + idx
	return res
}

func cacheRemap16Byte(index uint64) uint64 {
	const cacheLineSize = cacheLineSize / 2
	rawIndex := index & uint64(scqsize-1)
	cacheLineNum := (rawIndex) % (scqsize / uint64(cacheLineSize))
	cacheLineIdx := rawIndex / (scqsize / uint64(cacheLineSize))
	return cacheLineNum*uint64(cacheLineSize) + cacheLineIdx
}
