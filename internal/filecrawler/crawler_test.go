package crawler

import (
	"context"
	"crawler/internal/fs"
	"crawler/pkg/mocks"
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCancelContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)

	t.Cleanup(
		func() {
			cancel()
		},
	)

	var (
		dirs        = max(runtime.NumCPU(), 6)
		filesPerDir = max(runtime.NumCPU(), 6)
	)

	_, err := runWithErrors(
		ctx,
		t,
		Configuration{
			SearchWorkers:      dirs / 6,
			FileWorkers:        dirs * filesPerDir / 6,
			AccumulatorWorkers: dirs * filesPerDir / 6,
		},
		dirs,
		filesPerDir,
		&errorsConfig{
			openFilePanic: false,
			openFileError: false,
			fileReadError: false,
			dirReadPanic:  false,
			dirReadError:  false,
		},
	)

	require.ErrorContains(t, err, context.DeadlineExceeded.Error())
}

func TestInternalState(t *testing.T) {
	require.Zero(t, unsafe.Sizeof(crawlerImpl[int, int]{}))
}

func TestWithOsFileSystem(t *testing.T) {
	ctx := context.Background()

	rootDir, err := os.MkdirTemp(os.TempDir(), "*")
	require.NoError(t, err)

	t.Cleanup(func() {
		err = os.RemoveAll(rootDir)
		require.NoError(t, err)
	})

	dirs := make([]string, 0, 10)

	for i := 0; i < 10; i++ {
		d, err := os.MkdirTemp(rootDir, "*")
		require.NoError(t, err)

		dirs = append(dirs, d)

	}

	for _, d := range dirs {
		for i := 0; i < 10; i++ {
			f, err := os.CreateTemp(d, "*")
			require.NoError(t, err)

			_, err = f.WriteString(`{"data": 1}`)
			require.NoError(t, err)

			err = f.Close()
			require.NoError(t, err)
		}
	}

	c := New[TestType, TestAccumulator]()
	result, err := c.Collect(ctx, fs.NewOsFileSystem(), rootDir, Configuration{
		10,
		10,
		10,
	}, accum, combiner)

	require.NoError(t, err)
	require.EqualValues(t, 100, result.Sum)
}

func TestWorkers(t *testing.T) {
	ctx := context.Background()

	maximum := atomic.Int64{}
	ch := make(chan struct{})
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-ch:
				return
			default:
				maximum.Store(max(maximum.Load(), int64(runtime.NumGoroutine())))
			}
		}
	}()

	_, err := run(
		ctx,
		t,
		Configuration{
			SearchWorkers:      100,
			FileWorkers:        100,
			AccumulatorWorkers: 100,
		},
		10,
		10,
	)
	require.NoError(t, err)
	ch <- struct{}{}

	require.LessOrEqual(t, maximum.Load(), int64(330))
	wg.Wait()
}

func TestGeneralPerformance(t *testing.T) {
	first := testing.Benchmark(func(b *testing.B) {
		ctx := context.Background()

		var (
			dirs        = max(runtime.NumCPU(), 6)
			filesPerDir = max(runtime.NumCPU(), 6)
		)

		for i := 0; i < b.N; i++ {
			result, err := run(
				ctx,
				b,
				Configuration{
					SearchWorkers:      dirs,
					FileWorkers:        dirs * filesPerDir,
					AccumulatorWorkers: dirs * filesPerDir,
				},
				dirs,
				filesPerDir,
			)

			require.NoError(b, err)
			require.EqualValues(b, dirs*filesPerDir, result.Sum)
		}
	})

	another := testing.Benchmark(func(b *testing.B) {
		ctx := context.Background()

		var (
			dirs        = max(runtime.NumCPU(), 6)
			filesPerDir = max(runtime.NumCPU(), 6)
		)

		for i := 0; i < b.N; i++ {
			result, err := run(
				ctx,
				b,
				Configuration{
					SearchWorkers:      dirs / 6,
					FileWorkers:        (dirs * filesPerDir) / 6,
					AccumulatorWorkers: (dirs * filesPerDir) / 6,
				},
				dirs,
				filesPerDir,
			)

			require.NoError(b, err)
			require.EqualValues(b, dirs*filesPerDir, result.Sum)
		}
	})

	require.GreaterOrEqual(t, float64(another.NsPerOp())/float64(first.NsPerOp()), 2.3)
}

func TestSearchPerformance(t *testing.T) {
	first := testing.Benchmark(func(b *testing.B) {
		ctx := context.Background()

		var (
			dirs        = max(runtime.NumCPU(), 6)
			filesPerDir = max(runtime.NumCPU(), 6)
		)

		for i := 0; i < b.N; i++ {
			result, err := run(
				ctx,
				b,
				Configuration{
					SearchWorkers:      dirs,
					FileWorkers:        dirs * filesPerDir,
					AccumulatorWorkers: dirs * filesPerDir,
				},
				dirs,
				filesPerDir,
			)

			require.NoError(b, err)
			require.EqualValues(b, dirs*filesPerDir, result.Sum)
		}
	})

	another := testing.Benchmark(func(b *testing.B) {
		ctx := context.Background()

		var (
			dirs        = max(runtime.NumCPU(), 6)
			filesPerDir = max(runtime.NumCPU(), 6)
		)

		for i := 0; i < b.N; i++ {
			result, err := run(
				ctx,
				b,
				Configuration{
					SearchWorkers:      dirs / 6,
					FileWorkers:        dirs * filesPerDir,
					AccumulatorWorkers: dirs * filesPerDir,
				},
				dirs,
				filesPerDir,
			)

			require.NoError(b, err)
			require.EqualValues(b, dirs*filesPerDir, result.Sum)
		}
	})

	require.GreaterOrEqual(t, float64(another.NsPerOp())/float64(first.NsPerOp()), 2.3)
}

func TestFilePerformance(t *testing.T) {
	first := testing.Benchmark(func(b *testing.B) {
		ctx := context.Background()

		var (
			dirs        = max(runtime.NumCPU(), 6)
			filesPerDir = max(runtime.NumCPU(), 6)
		)

		for i := 0; i < b.N; i++ {
			result, err := run(
				ctx,
				b,
				Configuration{
					SearchWorkers:      dirs,
					FileWorkers:        dirs * filesPerDir,
					AccumulatorWorkers: dirs * filesPerDir,
				},
				dirs,
				filesPerDir,
			)

			require.NoError(b, err)
			require.EqualValues(b, dirs*filesPerDir, result.Sum)
		}
	})

	another := testing.Benchmark(func(b *testing.B) {
		ctx := context.Background()

		var (
			dirs        = max(runtime.NumCPU(), 6)
			filesPerDir = max(runtime.NumCPU(), 6)
		)

		for i := 0; i < b.N; i++ {
			result, err := run(
				ctx,
				b,
				Configuration{
					SearchWorkers:      dirs,
					FileWorkers:        (dirs * filesPerDir) / 6,
					AccumulatorWorkers: dirs * filesPerDir,
				},
				dirs,
				filesPerDir,
			)

			require.NoError(b, err)
			require.EqualValues(b, dirs*filesPerDir, result.Sum)
		}
	})

	require.GreaterOrEqual(t, float64(another.NsPerOp())/float64(first.NsPerOp()), 2.3)
}

func TestAccumPerformance(t *testing.T) {
	first := testing.Benchmark(func(b *testing.B) {
		ctx := context.Background()

		var (
			dirs        = max(runtime.NumCPU(), 6)
			filesPerDir = max(runtime.NumCPU(), 6)
		)

		for i := 0; i < b.N; i++ {
			result, err := run(
				ctx,
				b,
				Configuration{
					SearchWorkers:      dirs,
					FileWorkers:        dirs * filesPerDir,
					AccumulatorWorkers: dirs * filesPerDir,
				},
				dirs,
				filesPerDir,
			)

			require.NoError(b, err)
			require.EqualValues(b, dirs*filesPerDir, result.Sum)
		}
	})

	another := testing.Benchmark(func(b *testing.B) {
		ctx := context.Background()

		var (
			dirs        = max(runtime.NumCPU(), 6)
			filesPerDir = max(runtime.NumCPU(), 6)
		)

		for i := 0; i < b.N; i++ {
			result, err := run(
				ctx,
				b,
				Configuration{
					SearchWorkers:      dirs,
					FileWorkers:        dirs * filesPerDir,
					AccumulatorWorkers: (dirs * filesPerDir) / 6,
				},
				dirs,
				filesPerDir,
			)

			require.NoError(b, err)
			require.EqualValues(b, dirs*filesPerDir, result.Sum)
		}
	})

	require.GreaterOrEqual(t, float64(another.NsPerOp())/float64(first.NsPerOp()), 2.3)
}

func TestSingleWorker(t *testing.T) {
	ctx := context.Background()

	var (
		dirs        = 2
		filesPerDir = 2
	)

	result, err := run(
		ctx,
		t,
		Configuration{
			SearchWorkers:      1,
			FileWorkers:        1,
			AccumulatorWorkers: 1,
		},
		dirs,
		filesPerDir,
	)

	require.NoError(t, err)
	require.EqualValues(t, dirs*filesPerDir, result.Sum)
}

func TestErrorHandle(t *testing.T) {
	testCases := []struct {
		conf      *errorsConfig
		targetErr error
	}{
		{
			conf: &errorsConfig{
				openFilePanic: true,
			},
			targetErr: ErrFileOpenPanic,
		},
		{
			conf: &errorsConfig{
				openFileError: true,
			},
			targetErr: ErrFileOpen,
		},
		{
			conf: &errorsConfig{
				fileReadError: true,
			},
			targetErr: ErrFileRead,
		},
		{
			conf: &errorsConfig{
				dirReadPanic: true,
			},
			targetErr: ErrReadDirPanic,
		},
		{
			conf: &errorsConfig{
				dirReadError: true,
			},
			targetErr: ErrReadDir,
		},
	}

	for _, tt := range testCases {
		ctx := context.Background()

		var (
			dirs        = 2
			filesPerDir = 2
		)

		_, err := runWithErrors(
			ctx,
			t,
			Configuration{
				SearchWorkers:      1,
				FileWorkers:        1,
				AccumulatorWorkers: 3,
			},
			dirs,
			filesPerDir,
			tt.conf,
		)

		require.ErrorIs(t, err, tt.targetErr)
	}
}

const (
	sleepTime = time.Nanosecond * 100 * 10e6
)

var (
	ErrFileOpenPanic = errors.New("open file panic")
	ErrFileOpen      = errors.New("open file")
	ErrFileRead      = errors.New("read file")

	ErrReadDirPanic = errors.New("read dir panic")
	ErrReadDir      = errors.New("read dir")
)

type TestType struct {
	Data int64 `json:"data"`
}

type TestAccumulator struct {
	Sum int64 `json:"sum"`
}

func accum(current TestType, accum TestAccumulator) TestAccumulator {
	time.Sleep(sleepTime)

	accum.Sum += current.Data
	return accum
}

var global = make(map[int64]int)

func combiner(first, second TestAccumulator) TestAccumulator {
	second.Sum += first.Sum

	// check thread access
	global[first.Sum]++

	return second
}

func testCompilation[T, R any]() Crawler[T, R] {
	return &crawlerImpl[T, R]{}
}

type errorsConfig struct {
	openFilePanic bool
	openFileError bool
	fileReadError bool
	dirReadPanic  bool
	dirReadError  bool
}

func runWithErrors(
	ctx context.Context,
	t testing.TB,
	conf Configuration,
	dirs int,
	filesPerDir int,
	cfg *errorsConfig,
) (TestAccumulator, error) {
	controller := gomock.NewController(t)

	t.Cleanup(func() {
		controller.Finish()
	})

	root := "root"
	file := mocks.NewMockFile(controller)

	file.EXPECT().
		Read(gomock.Any()).
		DoAndReturn(func(p []byte) (n int, err error) {
			if cfg.fileReadError {
				return 0, ErrFileRead
			}

			// ok for tests
			return strings.NewReader(`{"data": 1}`).Read(p)
		}).
		AnyTimes()

	file.EXPECT().
		Close().
		DoAndReturn(func() error {
			if rand.N(2) == 0 {
				return errors.New("test")
			}

			return nil
		}).
		AnyTimes()

	fileSystem := mocks.NewMockFileSystem(controller)

	fileSystem.EXPECT().
		Join(gomock.Any()).
		DoAndReturn(func(elem ...string) string {
			return filepath.Join(elem...)
		}).
		AnyTimes()

	fileSystem.EXPECT().
		Open(gomock.Any()).
		DoAndReturn(
			func(name string) (fs.File, error) {
				if cfg.openFilePanic {
					panic(ErrFileOpenPanic)
				}

				if cfg.openFileError {
					return nil, ErrFileOpen
				}

				time.Sleep(sleepTime)
				return file, nil
			},
		).
		AnyTimes()

	mockDir := mocks.NewMockDirEntry(controller)
	mockDir.EXPECT().
		Name().
		DoAndReturn(func() string {
			return strconv.FormatInt(rand.N[int64](10e9), 10)
		}).
		AnyTimes()

	mockDir.EXPECT().
		IsDir().
		Return(true).
		AnyTimes()

	fileDirEntry := mocks.NewMockDirEntry(controller)
	fileDirEntry.EXPECT().
		Name().
		Return(strconv.FormatInt(rand.N[int64](10e9), 10)).
		AnyTimes()

	fileDirEntry.EXPECT().
		IsDir().
		Return(false).
		AnyTimes()

	fileSystem.EXPECT().
		ReadDir(root).
		DoAndReturn(func(name string) ([]os.DirEntry, error) {
			r := make([]os.DirEntry, 0)
			for i := 0; i < dirs; i++ {
				r = append(r, mockDir)
			}

			return r, nil
		}).Times(1)

	fileSystem.EXPECT().
		ReadDir(gomock.Any()).
		DoAndReturn(func(name string) ([]os.DirEntry, error) {
			time.Sleep(sleepTime)

			if cfg.dirReadPanic {
				panic(ErrReadDirPanic)
			}

			if cfg.dirReadError {
				return nil, ErrReadDir
			}

			r := make([]os.DirEntry, 0)
			for i := 0; i < filesPerDir; i++ {
				r = append(r, fileDirEntry)
			}

			return r, nil
		}).
		AnyTimes()

	c := New[TestType, TestAccumulator]()
	result, err := c.Collect(ctx, fileSystem, root, conf, accum, combiner)
	require.LessOrEqual(t, runtime.NumGoroutine(), 3)

	return result, err
}

func run(
	ctx context.Context,
	t testing.TB,
	conf Configuration,
	dirs int,
	filesPerDir int,
) (TestAccumulator, error) {
	controller := gomock.NewController(t)

	t.Cleanup(func() {
		controller.Finish()
	})

	root := "root"
	file := mocks.NewMockFile(controller)

	file.EXPECT().
		Read(gomock.Any()).
		DoAndReturn(func(p []byte) (n int, err error) {
			// ok for tests
			return strings.NewReader(`{"data": 1}`).Read(p)
		}).
		Times(dirs * filesPerDir)

	file.EXPECT().
		Close().
		DoAndReturn(func() error {
			if rand.N(2) == 0 {
				return errors.New("test")
			}

			return nil
		}).
		Times(dirs * filesPerDir)

	fileSystem := mocks.NewMockFileSystem(controller)

	fileSystem.EXPECT().
		Join(gomock.Any()).
		DoAndReturn(func(elem ...string) string {
			return filepath.Join(elem...)
		}).
		Times(dirs + dirs*filesPerDir)

	fileSystem.EXPECT().
		Open(gomock.Any()).
		DoAndReturn(
			func(name string) (fs.File, error) {
				time.Sleep(sleepTime)
				return file, nil
			},
		).
		Times(dirs * filesPerDir)

	mockDir := mocks.NewMockDirEntry(controller)
	mockDir.EXPECT().
		Name().
		DoAndReturn(func() string {
			return strconv.FormatInt(rand.N[int64](10e9), 10)
		}).Times(dirs)

	mockDir.EXPECT().
		IsDir().
		Return(true).
		Times(dirs)

	fileDirEntry := mocks.NewMockDirEntry(controller)
	fileDirEntry.EXPECT().
		Name().
		Return(strconv.FormatInt(rand.N[int64](10e9), 10)).
		Times(dirs * filesPerDir)

	fileDirEntry.EXPECT().
		IsDir().
		Return(false).
		Times(dirs * filesPerDir)

	fileSystem.EXPECT().
		ReadDir(root).
		DoAndReturn(func(name string) ([]os.DirEntry, error) {
			r := make([]os.DirEntry, 0)
			for i := 0; i < dirs; i++ {
				r = append(r, mockDir)
			}

			return r, nil
		}).Times(1)

	fileSystem.EXPECT().
		ReadDir(gomock.Any()).
		DoAndReturn(func(name string) ([]os.DirEntry, error) {
			time.Sleep(sleepTime)

			r := make([]os.DirEntry, 0)
			for i := 0; i < filesPerDir; i++ {
				r = append(r, fileDirEntry)
			}

			return r, nil
		}).Times(dirs)

	c := New[TestType, TestAccumulator]()
	result, err := c.Collect(ctx, fileSystem, root, conf, accum, combiner)
	require.LessOrEqual(t, runtime.NumGoroutine(), 3)

	return result, err
}
