package crawler

import (
	"context"
	"crawler/internal/fs"
	"crawler/internal/workerpool"
	"encoding/json"
	"sync"
)

// Configuration holds the configuration for the crawler, specifying the number of workers for
// file searching, processing, and accumulating tasks. The values for SearchWorkers, FileWorkers,
// and AccumulatorWorkers are critical to efficient performance and must be defined in
// every configuration.
type Configuration struct {
	SearchWorkers      int // Number of workers responsible for searching files.
	FileWorkers        int // Number of workers for processing individual files.
	AccumulatorWorkers int // Number of workers for accumulating results.
}

// Combiner is a function type that defines how to combine two values of type R into a single
// result. Combiner is not required to be thread-safe
//
// Combiner can either:
//   - Modify one of its input arguments to include the result of the other and return it,
//     or
//   - Create a new combined result based on the inputs and return it.
//
// It is assumed that type R has a neutral element (forming a monoid)
type Combiner[R any] func(current R, accum R) R

// Crawler represents a concurrent crawler implementing a map-reduce model with multiple workers
// to manage file processing, transformation, and accumulation tasks. The crawler is designed to
// handle large sets of files efficiently, assuming that all files can fit into memory
// simultaneously.
type Crawler[T, R any] interface {
	// Collect performs the full crawling operation, coordinating with the file system
	// and worker pool to process files and accumulate results. The result type R is assumed
	// to be a monoid, meaning there exists a neutral element for combination, and that
	// R supports an associative combiner operation.
	// The result of this collection process, after all reductions, is returned as type R.
	//
	// Important requirements:
	// 1. Number of workers in the Configuration is mandatory for managing workload efficiently.
	// 2. FileSystem and Accumulator must be thread-safe.
	// 3. Combiner does not need to be thread-safe.
	// 4. If an accumulator or combiner function modifies one of its arguments,
	//    it should return that modified value rather than creating a new one,
	//    or alternatively, it can create and return a new combined result.
	// 5. Context cancellation is respected across workers.
	// 6. Type T is derived by json-deserializing the file contents, and any issues in deserialization
	//    must be handled within the worker.
	// 7. The combiner function will wait for all workers to complete, ensuring no goroutine leaks
	//    occur during the process.
	Collect(
		ctx context.Context,
		fileSystem fs.FileSystem,
		root string,
		conf Configuration,
		accumulator workerpool.Accumulator[T, R],
		combiner Combiner[R],
	) (R, error)
}

type crawlerImpl[T, R any] struct{}

func New[T, R any]() *crawlerImpl[T, R] {
	return &crawlerImpl[T, R]{}
}

func (c *crawlerImpl[T, R]) Collect(
	ctx context.Context,
	fileSystem fs.FileSystem,
	root string,
	conf Configuration,
	accumulator workerpool.Accumulator[T, R],
	combiner Combiner[R],
) (R, error) {
	// pool for search dirs
	searchPool := workerpool.New[string, string]()
	// pool for transform chan files => chan T
	transformPoll := workerpool.New[string, T]()
	// pool for accumulate chan T
	accumulatePool := workerpool.New[T, R]()

	filesCh := make(chan string)
	wg := new(sync.WaitGroup)
	mu := new(sync.Mutex)

	var err error

	// file(string) => chan files(string)
	// fills the filesCh channel using the search function
	searchPool.List(ctx, conf.SearchWorkers, root, createSearcher(filesCh, fileSystem, wg, ctx, mu, &err))

	go func() {
		defer close(filesCh)
		wg.Wait()
	}()
	// chan file(string) => json => chan T
	// fills the data channel with transformed values
	data := transformPoll.Transform(ctx, conf.FileWorkers, filesCh, createTransformer[T](fileSystem, &err))

	// chan T => chan R
	// fills the channel with accumulated values from each worker
	accumulated := accumulatePool.Accumulate(ctx, conf.AccumulatorWorkers, data, accumulator)

	// chan R => ans R
	// accumulate the values from the channel into the ans using the combiner
	comb := <-accumulated
	for {
		el, ok := <-accumulated
		if !ok {
			break
		}
		comb = combiner(comb, el)
	}

	return comb, err
}

// createSearcher returns a function that takes a parent directory path
// and returns a slice of subdirectories. It reads the directory contents,
// separates files and directories, and sends file paths to the output channel.
func createSearcher(
	filesCh chan string,
	fileSystem fs.FileSystem,
	wg *sync.WaitGroup,
	ctx context.Context,
	mu *sync.Mutex,
	err *error) func(string) []string {
	return func(parent string) []string {
		defer func() {
			if r := recover(); r != nil {
				*err = r.(error)
			}
		}()

		// err
		arr, readDirErr := fileSystem.ReadDir(parent)
		if readDirErr != nil {
			*err = readDirErr
			return make([]string, 0)
		}

		dirs := make([]string, 0, len(arr))
		files := make([]string, 0, len(arr))

		for _, el := range arr {
			if el.IsDir() {
				dirs = append(dirs, fileSystem.Join(parent, el.Name()))
			} else {
				files = append(files, fileSystem.Join(parent, el.Name()))
			}
		}

		// send files in filesCh
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, el := range files {
				select {
				case <-ctx.Done():
					mu.Lock()
					*err = ctx.Err()
					mu.Unlock()

					return
				case filesCh <- el:
				}
			}
		}()
		return dirs
	}
}

// createTransformer returns a function that takes a file path, reads the file,
// and decodes its JSON content into a value of type T.
func createTransformer[T any](fileSystem fs.FileSystem, err *error) func(string) T {
	return func(el string) T {
		defer func() {
			if r := recover(); r != nil {
				*err = r.(error)
			}
		}()

		var myData T
		file, errOpenFile := fileSystem.Open(el)
		// err
		if errOpenFile != nil {
			*err = errOpenFile
			return myData
		}
		defer file.Close()

		// err
		decoder := json.NewDecoder(file)
		errReadFile := decoder.Decode(&myData)
		if errReadFile != nil {
			*err = errReadFile
			return myData
		}

		return myData
	}
}
