package queue

import (
	"log/slog"
	"sync"
)

type JobFunc func() error

type Queue struct {
	jobs chan JobFunc
	wg   sync.WaitGroup
}

func New(buffer int) *Queue {
	return &Queue{
		jobs: make(chan JobFunc, buffer),
	}
}

func (q *Queue) Start(workers int) {
	for i := 0; i < workers; i++ {
		q.wg.Add(1)

		go func() {
			defer q.wg.Done()

			for job := range q.jobs {
				if err := job(); err != nil {
					slog.Error("Background job failed", "error", err)
				}
			}
		}()
	}
}

func (q *Queue) Run(job JobFunc) {
	q.jobs <- job
}

func (q *Queue) Stop() {
	close(q.jobs)
	q.wg.Wait()
}
