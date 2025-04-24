package main

import (
	"context"
	crawler "crawler/internal/filecrawler"
	"crawler/internal/fs"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type TestType struct {
	Data int64 `json:"data"`
}

type TestAccumulator struct {
	Sum int64 `json:"sum"`
}

func accum(current TestType, accum TestAccumulator) TestAccumulator {
	time.Sleep(time.Second)
	accum.Sum += current.Data
	return accum
}

func combiner(first, second TestAccumulator) TestAccumulator {
	second.Sum += first.Sum
	return second
}

func main() {
	ctx := context.Background()
	wd, err := os.Getwd()

	if err != nil {
		panic(err)
	}

	root := filepath.Join(wd, "tests")
	fmt.Println(root)

	c := crawler.New[TestType, TestAccumulator]()
	result, err := c.Collect(ctx, fs.NewOsFileSystem(), root, crawler.Configuration{
		SearchWorkers:      10,
		FileWorkers:        10,
		AccumulatorWorkers: 10,
	}, accum, combiner)

	if err != nil {
		panic(err)
	}

	fmt.Println(result.Sum) // 300
}
