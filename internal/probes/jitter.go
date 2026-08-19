package probes

import (
	"math"
	"sync"
)

// JitterCalculator computes real-time instant latency jitter (|RTT_n - RTT_(n-1)|)
// strictly conforming to requirement FR5 (no statistical smoothing).
type JitterCalculator struct {
	mu      sync.Mutex
	lastRTT float64
	hasRTT  bool
}

// NewJitterCalculator creates an initialized JitterCalculator.
func NewJitterCalculator() *JitterCalculator {
	return &JitterCalculator{}
}

// Compute calculates the absolute difference between the new RTT and previous RTT.
// It returns (jitter, true) if a previous RTT exists, or (0, false) on the first measurement.
func (j *JitterCalculator) Compute(rttSeconds float64) (float64, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if !j.hasRTT {
		j.lastRTT = rttSeconds
		j.hasRTT = true
		return 0, false
	}

	jitter := math.Abs(rttSeconds - j.lastRTT)
	j.lastRTT = rttSeconds
	return jitter, true
}

// Reset clears the historical RTT value.
func (j *JitterCalculator) Reset() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.lastRTT = 0
	j.hasRTT = false
}
