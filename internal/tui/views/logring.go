package views

// lineRing is a fixed-capacity circular buffer of strings.
// When full, new writes overwrite the oldest entries (eviction).
// Not safe for concurrent use; callers must rely on the single-goroutine Bubble Tea model.
type lineRing struct {
	buf     []string
	cap     int
	head    int // index of the oldest entry
	size    int // number of valid entries (0..cap)
	evicted int // total lines dropped since last reset
}

// newLineRing allocates a ring with the given capacity.
// Panics if cap < 1.
func newLineRing(cap int) *lineRing {
	if cap < 1 {
		panic("lineRing: capacity must be >= 1")
	}
	return &lineRing{buf: make([]string, cap), cap: cap}
}

// push adds a line. When full, the oldest line is silently dropped and evicted is incremented.
func (r *lineRing) push(line string) {
	if r.size < r.cap {
		r.buf[(r.head+r.size)%r.cap] = line
		r.size++
	} else {
		// Overwrite the oldest slot and advance head.
		r.buf[r.head] = line
		r.head = (r.head + 1) % r.cap
		r.evicted++
	}
}

// pushAll appends a batch of lines efficiently.
func (r *lineRing) pushAll(lines []string) {
	for _, l := range lines {
		r.push(l)
	}
}

// reset clears all entries without reallocating the backing slice.
func (r *lineRing) reset() {
	r.head = 0
	r.size = 0
	r.evicted = 0
}

// len returns the number of valid entries.
func (r *lineRing) len() int { return r.size }

// isFull reports whether the buffer has reached capacity.
func (r *lineRing) isFull() bool { return r.size == r.cap }

// get returns the i-th oldest entry (0 = oldest, len-1 = newest).
// Panics if i is out of range.
func (r *lineRing) get(i int) string {
	if i < 0 || i >= r.size {
		panic("lineRing: index out of range")
	}
	return r.buf[(r.head+i)%r.cap]
}

// slice returns all valid entries in order from oldest to newest.
// The returned slice is a copy — safe to hold across mutations.
func (r *lineRing) slice() []string {
	out := make([]string, r.size)
	for i := range out {
		out[i] = r.buf[(r.head+i)%r.cap]
	}
	return out
}
