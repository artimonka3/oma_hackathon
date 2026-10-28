// Package store определяет границу транзакций и репозиториев.
//
// В каркасе используется InMemoryRunner: команды выполняются без общей
// транзакции, репозитории доменов реализованы в памяти процесса
// (internal/project, internal/prorab, internal/soberi, internal/mockoma).
// На этапе A ТЗ Runner заменяется на транзакции PostgreSQL, а репозитории —
// на SQL-реализации с теми же интерфейсами; доменный код не меняется.
package store

import "context"

// Tx — контракт транзакции доменной команды.
type Tx interface {
	Commit() error
	Rollback() error
}

// Runner открывает транзакцию для команды, которая атомарно сохраняет
// агрегат, новую версию и исходящее событие (раздел 9 ТЗ).
type Runner interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
}

// InMemoryRunner — каркасная реализация без общей транзакции.
type InMemoryRunner struct{}

// NewInMemoryRunner возвращает каркасный runner.
func NewInMemoryRunner() *InMemoryRunner { return &InMemoryRunner{} }

type noopTx struct{}

func (noopTx) Commit() error   { return nil }
func (noopTx) Rollback() error { return nil }

// WithinTx сразу выполняет fn.
func (InMemoryRunner) WithinTx(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error {
	return fn(ctx, noopTx{})
}
