// Package events — журнал доменных событий проекта.
//
// В каркасе журнал живёт в памяти. На этапе A (основа, PostgreSQL) запись
// события сохраняется в той же транзакции, что и доменная команда;
// сейчас интерфейс под это спроектирован.
package events

import (
	"sync"
	"time"

	"oma/internal/id"
)

// Event — запись журнала доменных событий проекта.
type Event struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Aggregate string    `json:"aggregate"`
	Type      string    `json:"type"`
	Version   int64     `json:"version"`
	Author    string    `json:"author"`
	Data      any       `json:"data,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Log — журнал событий с версионированием по проекту
// и защитой от повторной обработки входящих событий магазина.
type Log struct {
	mu   sync.Mutex
	next map[string]int64
	seen map[string]bool
	all  []Event
}

// NewLog создаёт пустой журнал.
func NewLog() *Log {
	return &Log{next: map[string]int64{}, seen: map[string]bool{}}
}

// Append добавляет доменное событие проекта.
func (l *Log) Append(projectID, aggregate, typ, author string, data any) Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.next[projectID]++
	e := Event{
		ID:        id.New("evt"),
		ProjectID: projectID,
		Aggregate: aggregate,
		Type:      typ,
		Version:   l.next[projectID],
		Author:    author,
		Data:      data,
		CreatedAt: time.Now().UTC(),
	}
	l.all = append(l.all, e)
	return e
}

// ByProject возвращает события проекта в порядке записи.
func (l *Log) ByProject(projectID string) []Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Event, 0, 4)
	for _, e := range l.all {
		if e.ProjectID == projectID {
			out = append(out, e)
		}
	}
	return out
}

// IngestExternal принимает входящее событие магазина по уникальному
// идентификатору: повторный идентификатор обрабатывается один раз
// и не создаёт второго эффекта (раздел 9 ТЗ).
func (l *Log) IngestExternal(eventID, typ, aggregate, aggregateID, projectID string, data any) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if eventID == "" || l.seen[eventID] {
		return false
	}
	l.seen[eventID] = true
	l.next[projectID]++
	l.all = append(l.all, Event{
		ID:        id.New("evt"),
		ProjectID: projectID,
		Aggregate: aggregate,
		Type:      typ,
		Version:   l.next[projectID],
		Author:    "oma_shop",
		Data:      data,
		CreatedAt: time.Now().UTC(),
	})
	_ = aggregateID
	return true
}
