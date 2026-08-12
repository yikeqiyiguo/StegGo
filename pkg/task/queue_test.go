package task

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunAllTasks(t *testing.T) {
	q := New(4)
	n := 20
	tasks := make([]Task, n)
	for i := range tasks {
		i := i
		tasks[i] = Task{ID: i, Name: fmt.Sprintf("task-%d", i), Fn: func() error {
			return nil
		}}
	}
	results := q.Run(context.Background(), tasks)
	if len(results) != n {
		t.Fatalf("结果数量应为 %d, 实际 %d", n, len(results))
	}
	for i, r := range results {
		if r.ID != i || r.Name != fmt.Sprintf("task-%d", i) {
			t.Fatalf("第 %d 个结果顺序错乱: %+v", i, r)
		}
		if r.Error != nil {
			t.Fatalf("第 %d 个任务失败: %v", i, r.Error)
		}
	}
}

func TestRunEmpty(t *testing.T) {
	q := New(4)
	results := q.Run(context.Background(), nil)
	if len(results) != 0 {
		t.Fatal("空任务应返回空结果")
	}
}

func TestRunCapturesErrors(t *testing.T) {
	q := New(2)
	tasks := []Task{
		{ID: 0, Name: "ok", Fn: func() error { return nil }},
		{ID: 1, Name: "fail", Fn: func() error { return errors.New("boom") }},
	}
	results := q.Run(context.Background(), tasks)
	if results[0].Error != nil {
		t.Fatalf("任务0不应失败: %v", results[0].Error)
	}
	if results[1].Error == nil || results[1].Error.Error() != "boom" {
		t.Fatalf("任务1应记录错误: %v", results[1].Error)
	}
}

func TestRunHonorsConcurrency(t *testing.T) {
	var concurrent int32
	var maxConcurrent int32
	q := New(3)
	tasks := make([]Task, 12)
	for i := range tasks {
		tasks[i] = Task{ID: i, Fn: func() error {
			cur := atomic.AddInt32(&concurrent, 1)
			for {
				m := atomic.LoadInt32(&maxConcurrent)
				if cur > m {
					if atomic.CompareAndSwapInt32(&maxConcurrent, m, cur) {
						break
					}
				} else {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt32(&concurrent, -1)
			return nil
		}}
	}
	q.Run(context.Background(), tasks)
	if maxConcurrent > 3 {
		t.Fatalf("并发度超出限制: %d > 3", maxConcurrent)
	}
}

func TestRunContextCancel(t *testing.T) {
	q := New(2)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消
	tasks := make([]Task, 10)
	for i := range tasks {
		tasks[i] = Task{ID: i, Fn: func() error { return nil }}
	}
	results := q.Run(ctx, tasks)
	// 取消后部分任务可能未执行，但结果必须完整且不 panic
	if len(results) != 10 {
		t.Fatalf("结果数量应为 10, 实际 %d", len(results))
	}
	for _, r := range results {
		_ = r
	}
}

func TestNewDefaultConcurrency(t *testing.T) {
	q := New(0)
	if q.concurrency != 4 {
		t.Fatalf("默认并发度应为 4, 实际 %d", q.concurrency)
	}
}
