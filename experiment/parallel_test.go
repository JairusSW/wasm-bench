package experiment

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestParallelHonorsWorkerLimitAndCompletesTasks(t *testing.T) {
	var active, peak atomic.Int32
	var completed atomic.Int32
	err := parallel(context.Background(), 3, 24, func(int) error {
		current := active.Add(1)
		defer active.Add(-1)
		for observed := peak.Load(); current > observed && !peak.CompareAndSwap(observed, current); observed = peak.Load() {
		}
		time.Sleep(time.Millisecond)
		completed.Add(1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if completed.Load() != 24 || peak.Load() != 3 {
		t.Fatalf("completed=%d peak_workers=%d", completed.Load(), peak.Load())
	}
}

func TestParallelStopsDispatchAfterTaskFailure(t *testing.T) {
	var started atomic.Int32
	err := parallel(context.Background(), 2, 100, func(index int) error {
		started.Add(1)
		if index == 1 {
			return fmt.Errorf("deliberate failure")
		}
		time.Sleep(time.Millisecond)
		return nil
	})
	if err == nil || started.Load() >= 100 {
		t.Fatalf("err=%v started=%d", err, started.Load())
	}
}

func TestParallelPhasesFinishInDeclaredOrder(t *testing.T) {
	phases := [][]int{{0, 0, 0}, {1, 1, 1}, {2, 2, 2}}
	var active, last atomic.Int32
	var outOfOrder atomic.Bool
	err := parallelPhases(context.Background(), 3, phases, func(phase int) error {
		current := active.Add(1)
		if current == 1 {
			last.Store(int32(phase))
		} else if last.Load() != int32(phase) {
			outOfOrder.Store(true)
		}
		time.Sleep(time.Millisecond)
		active.Add(-1)
		return nil
	})
	if err != nil || outOfOrder.Load() || active.Load() != 0 {
		t.Fatalf("err=%v out_of_order=%v active=%d", err, outOfOrder.Load(), active.Load())
	}
}
