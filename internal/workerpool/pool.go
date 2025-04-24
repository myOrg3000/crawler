package workerpool

import (
	"context"
	"sync"
)

// Accumulator is a function type used to aggregate values of type T into a result of type R.
// It must be thread-safe, as multiple goroutines will access the accumulator function concurrently.
// Each worker will produce intermediate results, which are combined with an initial or
// accumulated value.
type Accumulator[T, R any] func(current T, accum R) R

// Transformer is a function type used to transform an element of type T to another type R.
// The function is invoked concurrently by multiple workers, and therefore must be thread-safe
// to ensure data integrity when accessed across multiple goroutines.
// Each worker independently applies the transformer to its own subset of data, and although
// no shared state is expected, the transformer must handle any internal state in a thread-safe
// manner if present.
type Transformer[T, R any] func(current T) R

// Searcher is a function type for exploring data in a hierarchical manner.
// Each call to Searcher takes a parent element of type T and returns a slice of T representing
// its child elements. Since multiple goroutines may call Searcher concurrently, it must be
// thread-safe to ensure consistent results during recursive  exploration.
//
// Important considerations:
//  1. Searcher should be designed to avoid race conditions, particularly if it captures external
//     variables in closures.
//  2. The calling function must handle any state or values in closures, ensuring that
//     captured variables remain consistent throughout recursive or hierarchical search paths.
type Searcher[T any] func(parent T) []T

// Pool is the primary interface for managing worker pools, with support for three main
// operations: Transform, Accumulate, and List. Each operation takes an input channel, applies
// a transformation, accumulation, or list expansion, and returns the respective output.
type Pool[T, R any] interface {
	// Transform applies a transformer function to each item received from the input channel,
	// with results sent to the output channel. Transform operates concurrently, utilizing the
	// specified number of workers. The number of workers must be explicitly defined in the
	// configuration for this function to handle expected workloads effectively.
	// Since multiple workers may call the transformer function concurrently, it must be
	// thread-safe to prevent race conditions or unexpected results when handling shared or
	// internal state. Each worker independently applies the transformer function to its own
	// data subset.
	Transform(ctx context.Context, workers int, input <-chan T, transformer Transformer[T, R]) <-chan R

	// Accumulate applies an accumulator function to the items received from the input channel,
	// with results accumulated and sent to the output channel. The accumulator function must
	// be thread-safe, as multiple workers concurrently update the accumulated result.
	// The output channel will contain intermediate accumulated results as R
	Accumulate(ctx context.Context, workers int, input <-chan T, accumulator Accumulator[T, R]) <-chan R

	// List expands elements based on a searcher function, starting
	// from the given element. The searcher function finds child elements for each parent,
	// allowing exploration in a tree-like structure.
	// The number of workers should be configured based on the workload, ensuring each worker
	// independently processes assigned elements.
	List(ctx context.Context, workers int, start T, searcher Searcher[T])
}

type poolImpl[T, R any] struct{}

func New[T, R any]() *poolImpl[T, R] {
	return &poolImpl[T, R]{}
}

func (p *poolImpl[T, R]) Accumulate(
	ctx context.Context,
	workers int,
	input <-chan T,
	accumulator Accumulator[T, R],
) <-chan R {
	result := make(chan R)
	wg := new(sync.WaitGroup)

	// accumulate values with workers
	startWorkers(ctx, workers, wg, func() {
		// Initialize the accumulated value for each worker
		var accumulatedVal R
		processFunctionResult(ctx, input, func(val T) {
			// Accumulate the value using the accumulator function,
			// or exit if the context is done.
			select {
			case <-ctx.Done():
				return
			default:
				accumulatedVal = accumulator(val, accumulatedVal)
			}
		})
		// Send the accumulated result to the result channel from each worker
		// or exit if the context is done.
		select {
		case <-ctx.Done():
			return
		case result <- accumulatedVal:
		}
	})

	go func() {
		defer close(result)
		wg.Wait()
	}()

	return result
}

func (p *poolImpl[T, R]) List(
	ctx context.Context,
	workers int,
	start T,
	searcher Searcher[T]) {

	wg := new(sync.WaitGroup)
	mu := new(sync.Mutex)
	input := make(chan T)

	// directories for processing on the current layer
	currentDirs := make([]T, 1)
	// directories for processing on the next layer
	newDirs := make([]T, 0)
	currentDirs[0] = start

	// pass through all search layers
	for {
		startWorkers(ctx, workers, wg,
			func() {
				processFunctionResult(ctx, input, func(val T) {
					for _, el := range searcher(val) {
						select {
						case <-ctx.Done():
							return
						default:
							mu.Lock()
							newDirs = append(newDirs, el)
							mu.Unlock()
						}
					}
				})
			})
		// filling the input channel with values
		for _, el := range currentDirs {
			select {
			case <-ctx.Done():
				// If the context is completed, then to complete correctly,
				// we close the input channel, wait until all worker processes complete their work,
				close(input)
				wg.Wait()
				return
			case input <- el:
			}
		}

		// close input after sending all values
		close(input)
		// barrier on each passage layer
		wg.Wait()

		// if there are no directories on the next layer then return
		if len(newDirs) == 0 {
			return
		}

		// Reinitialize the input channel for the next layer
		input = make(chan T)
		// transfer of new directories to current directories
		currentDirs = make([]T, len(newDirs))
		copy(currentDirs, newDirs)
		// Reinitialize the newDirs for the next layer
		newDirs = make([]T, 0, len(currentDirs))
	}
}

func (p *poolImpl[T, R]) Transform(
	ctx context.Context,
	workers int,
	input <-chan T,
	transformer Transformer[T, R],
) <-chan R {
	result := make(chan R)
	wg := new(sync.WaitGroup)

	// accumulate values from input chan with workers
	startWorkers(ctx, workers, wg, func() {
		processFunctionResult(ctx, input, func(val T) {
			// Transform the value using the transformer function,
			// or exit if the context is done.
			select {
			case <-ctx.Done():
				return
			case result <- transformer(val):
			}
		})
	})

	go func() {
		defer close(result)
		wg.Wait()
	}()

	return result
}

// startWorkers initializes and starts the specified number of worker goroutines.
// Each worker executes the provided workerFunc function. WaitGroup ensures that the main goroutine
// will wait for all workers to complete.
func startWorkers(ctx context.Context, workers int, wg *sync.WaitGroup, workerFunc func()) {
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			workerFunc()
		}()
	}
}

// processFunctionResult reads values from the input channel and processes each value
// using the provided processFunc function. The function exits if the context
// is done or if the input channel is closed. This ensures that the processing
// stops gracefully in case of cancellation or completion.
func processFunctionResult[T any](ctx context.Context, input <-chan T, processFunc func(val T)) {
	for {
		select {
		case <-ctx.Done():
			return
		case val, ok := <-input:
			if !ok {
				return
			}
			processFunc(val)
		}
	}
}
