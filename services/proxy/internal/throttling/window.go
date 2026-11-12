package throttling

// window is a per-key ring of span counters covering one interval.
type window struct {
	spans []int
	total int
}

func newWindow(spans int) *window {
	return &window{spans: make([]int, spans)}
}

func (w *window) add(slot, n int) {
	w.spans[slot] += n
	w.total += n
}

// clear drops the counts in slot (the oldest span, about to be reused).
func (w *window) clear(slot int) {
	w.total -= w.spans[slot]
	w.spans[slot] = 0
}
