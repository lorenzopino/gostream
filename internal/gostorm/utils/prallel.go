package utils

import (
	"runtime"
	"sync"
)

func ParallelFor(begin, end int, fn func(i int)) {
	// V320: Bounded worker pool prevents OOM on large loops.
	sem := make(chan struct{}, runtime.NumCPU())
	var wg sync.WaitGroup
	for i := begin; i < end; i++ {
		sem <- struct{}{} // acquire
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }() // release
			fn(i)
		}(i)
	}
	wg.Wait()
}
