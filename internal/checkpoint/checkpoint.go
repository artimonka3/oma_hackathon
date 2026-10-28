package checkpoint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Component interface {
	Export() (json.RawMessage, error)
	Import(json.RawMessage) error
}
type Store struct {
	Namespace       string
	ImportNamespace string
	Path            string
	Components      map[string]Component
	Extra           any
}

func (s *Store) Save() error {
	data := map[string]json.RawMessage{}
	for name, c := range s.Components {
		b, e := c.Export()
		if e != nil {
			return e
		}
		data[name] = b
	}
	extra, e := json.Marshal(s.Extra)
	if e != nil {
		return e
	}
	data["application"] = extra
	b, e := json.Marshal(data)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(s.Path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(s.Path), ".oma-*.tmp")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(f.Name(), s.Path)
}
func (s *Store) Load() error {
	b, e := os.ReadFile(s.Path)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	var data map[string]json.RawMessage
	if e = json.Unmarshal(b, &data); e != nil {
		return fmt.Errorf("invalid checkpoint %s: %w", s.Path, e)
	}
	for name, c := range s.Components {
		if raw, ok := data[name]; ok {
			if e = c.Import(raw); e != nil {
				return fmt.Errorf("restore %s: %w", name, e)
			}
		}
	}
	if raw, ok := data["application"]; ok {
		return json.Unmarshal(raw, s.Extra)
	}
	return nil
}
