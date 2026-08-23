package filesystem

type writeRange struct {
	start int
	end   int
}

type pendingWriteBlock struct {
	data  []byte
	dirty []writeRange
}

func (block *pendingWriteBlock) write(start int, data []byte) {
	copy(block.data[start:], data)
	merged := writeRange{start: start, end: start + len(data)}
	ranges := make([]writeRange, 0, len(block.dirty)+1)
	inserted := false
	for _, current := range block.dirty {
		switch {
		case current.end < merged.start:
			ranges = append(ranges, current)
		case merged.end < current.start:
			if !inserted {
				ranges = append(ranges, merged)
				inserted = true
			}
			ranges = append(ranges, current)
		default:
			merged.start = min(merged.start, current.start)
			merged.end = max(merged.end, current.end)
		}
	}
	if !inserted {
		ranges = append(ranges, merged)
	}
	block.dirty = ranges
}
