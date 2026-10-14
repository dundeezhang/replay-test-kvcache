package metrics

import (
	"sort"
	"sync"
)

// DefaultLatencyBuckets are upper bounds in seconds, tuned for an
// in-memory cache where most requests finish well under a millisecond.
var DefaultLatencyBuckets = []float64{
	0.0001, 0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 1,
}

// Histogram counts observations into fixed buckets. Bucket i counts
// observations v with upper[i-1] < v <= upper[i]; a final implicit bucket
// counts everything above the largest bound.
type Histogram struct {
	upper []float64

	mu     sync.Mutex
	counts []uint64
	sum    float64
	count  uint64
}

// NewHistogram returns a histogram with the given upper bounds. The bounds
// are copied and sorted.
func NewHistogram(upper []float64) *Histogram {
	b := append([]float64(nil), upper...)
	sort.Float64s(b)
	return &Histogram{upper: b, counts: make([]uint64, len(b)+1)}
}

// Observe records one value.
func (h *Histogram) Observe(v float64) {
	i := sort.Search(len(h.upper), func(i int) bool { return h.upper[i] > v })

	h.mu.Lock()
	h.counts[i]++
	h.sum += v
	h.count++
	h.mu.Unlock()
}

// snapshot returns cumulative bucket counts (one per upper bound, plus the
// overflow bucket), the sum of observations and their count.
func (h *Histogram) snapshot() (cumulative []uint64, sum float64, count uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	cumulative = make([]uint64, len(h.counts))
	var running uint64
	for i, c := range h.counts {
		running += c
		cumulative[i] = running
	}
	return cumulative, h.sum, h.count
}
