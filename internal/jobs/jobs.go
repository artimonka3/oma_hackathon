// Package jobs — сохранённая очередь заданий (AI, медиа, доставка событий).
//
// Обработчик (worker) забирает задания из очереди; состояния соответствуют
// разделу 3 технической спецификации. В каркасе очередь живёт в памяти
// процесса; на этапе A она переносится в PostgreSQL, и cmd/worker начинает
// обрабатывать задания app без изменения контрактов.
package jobs

import (
	"context"
	"log"
	"sync"
	"time"

	"oma/internal/apperr"
	"oma/internal/id"
)

// State — состояние задания.
type State string

const (
	StateQueued    State = "queued"
	StateRunning   State = "running"
	StateSucceeded State = "succeeded"
	StateFailed    State = "failed"
	StateCancelled State = "cancelled"
)

// Job — сохранённое задание.
type Job struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ProjectID string    `json:"project_id"`
	Input     any       `json:"input,omitempty"`
	State     State     `json:"state"`
	Result    any       `json:"result,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Queue — очередь заданий в памяти процесса.
type Queue struct {
	mu    sync.Mutex
	jobs  map[string]*Job
	order []string
}

// NewQueue создаёт пустую очередь.
func NewQueue() *Queue { return &Queue{jobs: map[string]*Job{}} }

// Enqueue добавляет задание в состояние queued.
func (q *Queue) Enqueue(kind, projectID string, input any) *Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	j := &Job{
		ID:        id.New("job"),
		Kind:      kind,
		ProjectID: projectID,
		Input:     input,
		State:     StateQueued,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	q.jobs[j.ID] = j
	q.order = append(q.order, j.ID)
	cp := *j
	return &cp
}

// Get возвращает копию задания.
func (q *Queue) Get(jobID string) (*Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	j, ok := q.jobs[jobID]
	if !ok {
		return nil, apperr.NotFound("задание не найдено")
	}
	cp := *j
	return &cp, nil
}

// Claim забирает следующее задание в состоянии queued и переводит в running.
func (q *Queue) Claim() (*Job, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, jid := range q.order {
		if q.jobs[jid].State == StateQueued {
			q.jobs[jid].State = StateRunning
			q.jobs[jid].UpdatedAt = time.Now().UTC()
			cp := *q.jobs[jid]
			return &cp, true
		}
	}
	return nil, false
}

// Complete помечает задание успешным и сохраняет результат.
func (q *Queue) Complete(jobID string, result any) {
	q.mu.Lock()
	j, ok := q.jobs[jobID]
	q.mu.Unlock()
	if !ok {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	j.State = StateSucceeded
	j.Result = result
	j.UpdatedAt = time.Now().UTC()
}

// Fail помечает задание неудачным; прежний подтверждённый проект остаётся доступен.
func (q *Queue) Fail(jobID, errMsg string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if j, ok := q.jobs[jobID]; ok {
		j.State = StateFailed
		j.Error = errMsg
		j.UpdatedAt = time.Now().UTC()
	}
}

// Handler обрабатывает одно задание; результат сохраняется в job.
type Handler func(ctx context.Context, job *Job) (any, error)

// Processor — цикл обработки сохранённых заданий.
type Processor struct {
	queue    *Queue
	handlers map[string]Handler
}

// NewProcessor создаёт обработчик для известных типов заданий.
func NewProcessor(q *Queue, handlers map[string]Handler) *Processor {
	return &Processor{queue: q, handlers: handlers}
}

// Run запускает цикл до отмены контекста.
func (p *Processor) Run(ctx context.Context, poll time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		job, ok := p.queue.Claim()
		if !ok {
			select {
			case <-ctx.Done():
				return
			case <-time.After(poll):
			}
			continue
		}
		handler, known := p.handlers[job.Kind]
		if !known {
			p.queue.Fail(job.ID, "нет обработчика для типа "+job.Kind)
			continue
		}
		res, err := handler(ctx, job)
		if err != nil {
			log.Printf("jobs: задание %s (%s) завершилось ошибкой: %v", job.ID, job.Kind, err)
			p.queue.Fail(job.ID, err.Error())
			continue
		}
		log.Printf("jobs: задание %s (%s) выполнено", job.ID, job.Kind)
		p.queue.Complete(job.ID, res)
	}
}
