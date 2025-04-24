package workerpool

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"
)

const (
	sleepTime = time.Second
)

type TestType struct {
	Data int64 `json:"data"`
}

type TestAccumulator struct {
	Sum int64 `json:"sum"`
}

func testCompilation[T, R any]() Pool[T, R] {
	return &poolImpl[T, R]{}
}

func generate[T any](s []T) <-chan T {
	result := make(chan T)

	go func() {
		defer close(result)

		for i := 0; i < len(s); i++ {
			result <- s[i]
		}
	}()

	return result
}

func collect[T any](in <-chan T) []T {
	result := make([]T, 0)

	for e := range in {
		result = append(result, e)
	}

	return result
}

func collectRestricted[T any](in <-chan T, restrict int) ([]T, error) {
	result := make([]T, 0)
	count := 0

	for e := range in {
		count++
		result = append(result, e)
	}

	if count > restrict {
		return []T{}, fmt.Errorf(
			"Exceeded in channel total elements restriction: expected %d, got %d",
			restrict,
			count,
		)
	}

	return result, nil
}

func accumulate(current TestType, accum TestType) TestType {
	time.Sleep(sleepTime)

	accum.Data += current.Data
	return accum
}

func transform(current TestType) TestType {
	time.Sleep(sleepTime)

	current.Data++
	return current
}

func TestInternalState(t *testing.T) {
	require.Zero(t, unsafe.Sizeof(poolImpl[int, int]{}))
}

func TestList(t *testing.T) {
	ctx := context.Background()
	wp := New[TestType, TestType]()

	start := TestType{Data: 123}
	inner := make([]TestType, 10)

	counter := atomic.Int64{}
	searcher := func(parent TestType) []TestType {
		counter.Add(1)
		time.Sleep(sleepTime)

		if parent == start {
			return inner
		}

		return []TestType{}
	}

	wp.List(ctx, 10, start, searcher)
	require.EqualValues(t, 11, counter.Load())
	require.LessOrEqual(t, runtime.NumGoroutine(), 3)
}

func TestListContextDone(t *testing.T) {
	t.Run("end", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*50)
		t.Cleanup(func() {
			cancel()
		})

		wp := New[TestType, TestType]()
		start := TestType{Data: 123}
		inner := make([]TestType, 10)

		searcher := func(parent TestType) []TestType {
			time.Sleep(time.Second * 5)

			if parent == start {
				return inner
			}

			time.Sleep(time.Hour)

			return []TestType{}
		}

		wp.List(ctx, 10, start, searcher)
	})

	t.Run("root", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		wp := New[TestType, TestType]()
		start := TestType{Data: 123}
		searcher := func(parent TestType) []TestType {
			time.Sleep(time.Hour)
			return []TestType{}
		}

		wp.List(ctx, 10, start, searcher)
	})
}

func TestListPerformance(t *testing.T) {
	first := testing.Benchmark(func(b *testing.B) {
		ctx := context.Background()
		wp := New[TestType, TestType]()

		start := TestType{Data: 123}
		inner := make([]TestType, 5)

		counter := atomic.Int64{}
		searcher := func(parent TestType) []TestType {
			counter.Add(1)
			time.Sleep(sleepTime)

			if parent == start {
				return inner
			}

			return []TestType{}
		}

		wp.List(ctx, 6, start, searcher)
	})

	second := testing.Benchmark(func(b *testing.B) {
		ctx := context.Background()
		wp := New[TestType, TestType]()

		start := TestType{Data: 123}
		inner := make([]TestType, 5)

		counter := atomic.Int64{}
		searcher := func(parent TestType) []TestType {
			counter.Add(1)
			time.Sleep(sleepTime)

			if parent == start {
				return inner
			}

			return []TestType{}
		}

		wp.List(ctx, 1, start, searcher)
	})

	require.GreaterOrEqual(t, float64(second.NsPerOp())/float64(first.NsPerOp()), 2.)
}

func TestAccumulate(t *testing.T) {
	ctx := context.Background()
	wp := New[TestType, TestType]()

	s := make([]TestType, 0, 10)
	for i := 0; i < 10; i++ {
		s = append(s, TestType{Data: 1})
	}

	in := generate(s)
	out := wp.Accumulate(ctx, 10, in, accumulate)
	result := collect(out)
	require.LessOrEqual(t, runtime.NumGoroutine(), 3)

	var sum int64
	for _, e := range result {
		sum += e.Data
	}

	require.EqualValues(t, 10, sum)
}

func TestAccumulatePerformance(t *testing.T) {
	first := testing.Benchmark(func(b *testing.B) {
		ctx := context.Background()
		wp := New[TestType, TestType]()

		s := make([]TestType, 0, 5)
		for i := 0; i < 5; i++ {
			s = append(s, TestType{Data: 1})
		}

		in := generate(s)
		_, err := collectRestricted(wp.Accumulate(ctx, 5, in, accumulate), 5)
		require.NoError(t, err)
	})

	second := testing.Benchmark(func(b *testing.B) {
		ctx := context.Background()
		wp := New[TestType, TestType]()

		s := make([]TestType, 0, 5)
		for i := 0; i < 5; i++ {
			s = append(s, TestType{Data: 1})
		}

		in := generate(s)
		_, err := collectRestricted(wp.Accumulate(ctx, 1, in, accumulate), 1)
		require.NoError(t, err)
	})

	require.GreaterOrEqual(t, float64(second.NsPerOp())/float64(first.NsPerOp()), 4.5)
}

func TestAccumulateContextDone(t *testing.T) {
	t.Run("input", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(func() {
			cancel()
		})

		wp := New[TestType, TestType]()

		in := make(chan TestType)
		wp.Accumulate(ctx, 10, in, accumulate)
		time.Sleep(time.Second)

		cancel()
	})

	t.Run("output", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(func() {
			cancel()
		})

		wp := New[TestType, TestType]()
		in := make(chan TestType, 10)

		for i := 0; i < 10; i++ {
			in <- TestType{}
		}
		close(in)

		wp.Accumulate(ctx, 100, in, accumulate)

		time.Sleep(1 * time.Second)
		cancel()
		time.Sleep(10 * time.Second)

		require.LessOrEqual(t, runtime.NumGoroutine(), 4)
	})
}

func TestTransform(t *testing.T) {
	ctx := context.Background()
	wp := New[TestType, TestType]()

	in := generate(make([]TestType, 10))
	out := wp.Transform(ctx, 10, in, transform)

	result := collect(out)
	require.Equal(t, 10, len(result))
	require.LessOrEqual(t, runtime.NumGoroutine(), 3)

	eq := TestType{Data: 1}
	for _, e := range result {
		require.EqualValues(t, eq, e)
	}
}

func TestTransformPerformance(t *testing.T) {
	first := testing.Benchmark(func(b *testing.B) {
		ctx := context.Background()
		wp := New[TestType, TestType]()

		for i := 0; i < b.N; i++ {
			in := generate(make([]TestType, 5))
			collect(wp.Transform(ctx, 5, in, transform))
		}
	})

	second := testing.Benchmark(func(b *testing.B) {
		ctx := context.Background()
		wp := New[TestType, TestType]()

		for i := 0; i < b.N; i++ {
			in := generate(make([]TestType, 5))
			collect(wp.Transform(ctx, 1, in, transform))
		}
	})

	require.GreaterOrEqual(t, float64(second.NsPerOp())/float64(first.NsPerOp()), 4.5)
}

func TestTransformContextDone(t *testing.T) {
	t.Run("input", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(func() {
			cancel()
		})

		wp := New[TestType, TestType]()

		in := make(chan TestType)
		out := wp.Transform(ctx, 10, in, transform)
		time.Sleep(time.Second)

		cancel()

		eq := TestType{Data: 1}
		for e := range out {
			fmt.Println(1)
			require.LessOrEqual(t, eq, e)
		}
	})

	t.Run("output", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(func() {
			cancel()
		})

		wp := New[TestType, TestType]()

		in := generate(make([]TestType, 10000))
		wp.Transform(ctx, 1, in, transform)
		time.Sleep(time.Second)

		cancel()
	})
}
