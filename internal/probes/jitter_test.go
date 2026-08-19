package probes_test

import (
	"math"
	"sync"
	"testing"

	"github.com/devleesch001/stabsight/internal/probes"
)

func TestJitterCalculator_Sequence(t *testing.T) {
	calc := probes.NewJitterCalculator()

	// First measurement: no jitter yet
	j1, ok1 := calc.Compute(0.010) // 10ms
	if ok1 {
		t.Errorf("expected ok=false on first measurement, got ok=true (jitter: %v)", j1)
	}

	// Second measurement: |15ms - 10ms| = 5ms
	j2, ok2 := calc.Compute(0.015)
	if !ok2 {
		t.Fatal("expected ok=true on second measurement")
	}
	if math.Abs(j2-0.005) > 1e-9 {
		t.Errorf("expected jitter 0.005, got %v", j2)
	}

	// Third measurement: |12ms - 15ms| = 3ms
	j3, ok3 := calc.Compute(0.012)
	if !ok3 {
		t.Fatal("expected ok=true on third measurement")
	}
	if math.Abs(j3-0.003) > 1e-9 {
		t.Errorf("expected jitter 0.003, got %v", j3)
	}

	// Reset
	calc.Reset()
	j4, ok4 := calc.Compute(0.020)
	if ok4 {
		t.Errorf("expected ok=false after reset, got ok=true (jitter: %v)", j4)
	}
}

func TestJitterCalculator_Concurrent(_ *testing.T) {
	calc := probes.NewJitterCalculator()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(val float64) {
			defer wg.Done()
			_, _ = calc.Compute(val)
		}(float64(i) * 0.001)
	}
	wg.Wait()
}
