# Crawler

## Задание
В данном домашнем задании вам необходимо реализовать многопоточный map-reduce crawler файлов.
Концептуальный пример использования можно посмотреть в [app.go](./cmd/app/app.go) для [tests](/tests)

Для его реализации вам необходимо поддержать вспомогательный интерфейс 

```go
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
```
Примеры использования можете посмотреть в [тестах](/internal/workerpool/pool_test.go)

Далее можете приступить к crawler:
```go
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
    Collect(
        ctx context.Context,
        fileSystem fs.FileSystem,
        root string,
        conf Configuration,
        accumulator workerpool.Accumulator[T, R],
        combiner Combiner[R],
    ) (R, error)
}
```

Примеры использования можете посмотреть в [тестах](/internal/filecrawler/crawler_test.go)

Для удобства тестирования был добавлен интерфейс [файловой системы](/internal/fs/filesystem.go) с готовыми реализациями.

## Особенности реализации

- Для простоты в `Pool` можно не обрабатывать ошибки, этим занимается `Crawler`
- Разрешается реализовать `List` с барьером на каждом слое прохода
- В самой реализации разрешается создавать вспомогательные воркеры для закрытия каналов
- Требуется корректная обработка ситуации отмены контекста
- Используйте тесты чтобы понять недосказанности
- В этом домашнем задании **запрещено** использовать буферизированные каналы

## Сдача
* Все функции реализовать в файлах [pool.go](/internal/workerpool/pool.go) и [crawler.go](/internal/filecrawler/crawler.go)
* Открыть pull request из ветки `hw` в ветку `main` **вашего репозитория**.
* В описании PR заполнить количество часов, которые вы потратили на это задание.
* Отправить заявку на ревью в соответствующей форме.
* Время дедлайна фиксируется отправкой формы.
* Изменять файлы в ветке main без PR запрещено.

## Makefile

Для удобств локальной разработки сделан [`Makefile`](Makefile). Имеются следующие команды:

Запустить полный цикл (линтер, тесты):

```bash 
make all
```

Запустить только тесты:

```bash
make test
``` 

Запустить линтер:

```bash
make lint
```

Подтянуть новые тесты:

```bash
make update
```

При разработке на Windows рекомендуется использовать [WSL](https://learn.microsoft.com/en-us/windows/wsl/install), чтобы
была возможность пользоваться вспомогательными скриптами.
