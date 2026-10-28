// Package media — интерфейс хранилища медиа.
//
// В разработке используется локальный каталог (MEDIA_STORAGE_PATH);
// при развёртывании подключается объектное хранилище без изменения
// доменного кода. Доступ к файлам проверяется по связи с проектом.
package media

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"oma/internal/apperr"
	"oma/internal/id"
)

// Meta — описание сохранённого медиа.
type Meta struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Author      string    `json:"author"`
	Kind        string    `json:"kind"` // photo | plan | device_scan | demo_capture
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	Size        int64     `json:"size"`
	CreatedAt   time.Time `json:"created_at"`
}

var allowedKinds = map[string]bool{
	"photo":        true,
	"plan":         true,
	"device_scan":  true,
	"demo_capture": true,
}

// Storage — интерфейс хранилища медиа.
type Storage interface {
	Save(ctx context.Context, m Meta, r io.Reader) (Meta, error)
}

// LocalStorage сохраняет файлы в локальном каталоге.
type LocalStorage struct {
	root string
}

// NewLocalStorage создаёт каталог хранилища, если он отсутствует.
func NewLocalStorage(root string) (*LocalStorage, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &LocalStorage{root: root}, nil
}

// Save проверяет тип и связь с проектом и записывает файл.
func (s *LocalStorage) Save(ctx context.Context, m Meta, r io.Reader) (Meta, error) {
	if !allowedKinds[m.Kind] {
		return Meta{}, apperr.InvalidInput("недопустимый тип медиа: "+m.Kind, nil)
	}
	if strings.TrimSpace(m.ProjectID) == "" {
		return Meta{}, apperr.InvalidInput("медиа обязано быть связано с проектом", nil)
	}
	m.ID = id.New("media")
	name := filepath.Join(s.root, m.ID+"_"+filepath.Base(m.Filename))
	f, err := os.Create(name)
	if err != nil {
		return Meta{}, err
	}
	defer f.Close()
	n, err := io.Copy(f, r)
	if err != nil {
		return Meta{}, err
	}
	m.Size = n
	m.CreatedAt = time.Now().UTC()
	return m, nil
}
