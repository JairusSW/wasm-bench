package experiment

import (
	"context"
	"fmt"
	"sync"
)

// parallel runs indexed, independent tasks with a fixed concurrency bound.
// The caller assigns stable output IDs before dispatching tasks.
func parallel(ctx context.Context, workers, count int, fn func(int) error) error {
	if count == 0 {
		return nil
	}
	if workers < 1 {
		workers = 1 // locks written before worker-count support
	}
	if workers > count {
		workers = count
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int)
	type taskError struct {
		index int
		err   error
	}
	errors := make(chan taskError, count)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			for index := range jobs {
				if ctx.Err() != nil {
					continue
				}
				if err := fn(index); err != nil {
					errors <- taskError{index: index, err: err}
					cancel()
				}
			}
		}()
	}
feed:
	for index := range count {
		select {
		case jobs <- index:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	group.Wait()
	close(errors)
	var first *taskError
	for failure := range errors {
		if first == nil || failure.index < first.index {
			copy := failure
			first = &copy
		}
	}
	if first != nil {
		return fmt.Errorf("task %d: %w", first.index, first.err)
	}
	return ctx.Err()
}

// parallelPhases keeps parallelism within a phase while ensuring every task
// in one ordered measurement stage completes before the next stage begins.
func parallelPhases[T any](ctx context.Context, workers int, phases [][]T, fn func(T) error) error {
	for _, phase := range phases {
		err := parallel(ctx, workers, len(phase), func(index int) error { return fn(phase[index]) })
		if err != nil {
			return err
		}
	}
	return nil
}
