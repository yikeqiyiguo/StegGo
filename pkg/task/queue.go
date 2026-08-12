// Package task 提供通用并发任务队列，供批量隐写、GUI 后台任务等复用。
package task

import (
	"context"
	"sync"
)

// Task 一个可执行单元。
type Task struct {
	ID   int
	Name string
	Fn   func() error
}

// Result 任务执行结果。
type Result struct {
	ID    int
	Name  string
	Error error
}

// Queue 并发任务队列。
type Queue struct {
	concurrency int
}

// New 创建任务队列。
func New(concurrency int) *Queue {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &Queue{concurrency: concurrency}
}

// Run 并发执行全部任务，按输入顺序返回结果。
// ctx 取消时停止派发新任务，已运行任务允许完成。
func (q *Queue) Run(ctx context.Context, tasks []Task) []Result {
	results := make([]Result, len(tasks))
	if len(tasks) == 0 {
		return results
	}
	idx := make(chan int)
	go func() {
		defer close(idx)
		for i := range tasks {
			select {
			case idx <- i:
			case <-ctx.Done():
				return
			}
		}
	}()

	sem := make(chan struct{}, q.concurrency)
	var wg sync.WaitGroup
	for i := range idx {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i] = Result{ID: tasks[i].ID, Name: tasks[i].Name, Error: ctx.Err()}
				return
			}
			defer func() { <-sem }()
			results[i] = Result{ID: tasks[i].ID, Name: tasks[i].Name, Error: tasks[i].Fn()}
		}(i)
	}
	wg.Wait()
	return results
}
