package workflow

import (
	"context"
	"fmt"
	"sync"
)

// ParallelResult holds the outcome of one activity in a RunParallel call.
type ParallelResult struct {
	Key    string         // the key passed to RunParallel
	Output map[string]any // nil on error
	Err    error          // nil on success
}

// RunParallel executes named activities concurrently, collecting all results.
// Each ActivityFunc receives the same input map (read-only — do not mutate it).
// Returns when all activities complete or ctx is cancelled.
// Results are returned in an unspecified order.
//
// Example — transcribe several videos concurrently:
//
//	fns := map[string]workflow.ActivityFunc{
//	    "video-1": transcribeFn,
//	    "video-2": transcribeFn,
//	}
//	for _, r := range workflow.RunParallel(ctx, fns, input) {
//	    if r.Err != nil { ... }
//	    fmt.Println(r.Key, r.Output["transcript"])
//	}
func RunParallel(ctx context.Context, fns map[string]ActivityFunc, input map[string]any) []ParallelResult {
	results := make([]ParallelResult, 0, len(fns))
	ch := make(chan ParallelResult, len(fns))

	var wg sync.WaitGroup
	for key, fn := range fns {
		wg.Add(1)
		go func(k string, f ActivityFunc) {
			defer wg.Done()
			out, err := f(ctx, input)
			ch <- ParallelResult{Key: k, Output: out, Err: err}
		}(key, fn)
	}

	wg.Wait()
	close(ch)

	for r := range ch {
		results = append(results, r)
	}
	return results
}

// RunParallelMerged executes named activities concurrently and merges all outputs
// into a single map. Later-finishing activities win on key collision.
// Returns the first non-nil error encountered (all activities still complete).
//
// Use this when you want to fan-out work and collect the merged result in one step,
// without caring which activity wrote which key.
func RunParallelMerged(ctx context.Context, fns map[string]ActivityFunc, input map[string]any) (map[string]any, error) {
	results := RunParallel(ctx, fns, input)

	merged := make(map[string]any)
	var firstErr error

	// Use a mutex because results come back in arbitrary order and we want a
	// deterministic merge without races if this function is ever called from multiple goroutines.
	var mu sync.Mutex
	for _, r := range results {
		mu.Lock()
		if r.Err != nil && firstErr == nil {
			firstErr = r.Err
		}
		for k, v := range r.Output {
			merged[k] = v
		}
		mu.Unlock()
	}

	return merged, firstErr
}

// FanOut creates a map of ActivityFuncs by applying makeFunc once per item in items.
// The returned map key is the item itself (converted to string with fmt.Sprintf).
// Use with RunParallel or RunParallelMerged to process a slice concurrently.
//
// Example:
//
//	fns := workflow.FanOut(urls, func(url string) workflow.ActivityFunc {
//	    return func(ctx context.Context, input map[string]any) (map[string]any, error) {
//	        in := maps.Clone(input)
//	        in["youtube_url"] = url
//	        return transcribeFn(ctx, in)
//	    }
//	})
//	results := workflow.RunParallel(ctx, fns, baseInput)
func FanOut[T any](items []T, makeFunc func(T) ActivityFunc) map[string]ActivityFunc {
	fns := make(map[string]ActivityFunc, len(items))
	for i, item := range items {
		key := itemKey(i, item)
		fns[key] = makeFunc(item)
	}
	return fns
}

func itemKey[T any](i int, item T) string {
	switch v := any(item).(type) {
	case string:
		if v != "" {
			return v
		}
	}
	return fmt.Sprintf("item_%d", i)
}
