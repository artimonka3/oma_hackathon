package media

import (
	"context"
	"encoding/json"
	"io"
	"oma/internal/apperr"
	"os"
	"path/filepath"
	"strings"
)

func (s *LocalStorage) Open(ctx context.Context, id string) (Meta, io.ReadCloser, error) {
	var m Meta
	if strings.ContainsAny(id, "/\\.") {
		return m, nil, apperr.NotFound("Фото не найдено")
	}
	raw, e := os.ReadFile(filepath.Join(s.root, id+".json"))
	if e != nil {
		return m, nil, apperr.NotFound("Фото не найдено")
	}
	if e = json.Unmarshal(raw, &m); e != nil {
		return m, nil, e
	}
	f, e := os.Open(filepath.Join(s.root, id+"_"+filepath.Base(m.Filename)))
	return m, f, e
}
