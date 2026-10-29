package checkpoint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"hash/fnv"
	"sync"
	"time"
)

// Postgres persists the existing domain snapshots in a single SQL transaction.
// It holds a database advisory lock: the in-process repositories support one writer.
type Postgres struct {
	conn   *pgx.Conn
	mu     sync.Mutex
	source *Store
}

func OpenPostgres(ctx context.Context, url string, source *Store) (*Postgres, error) {
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return nil, errors.New("PostgreSQL недоступен: проверьте DATABASE_URL и docker compose")
	}
	p := &Postgres{conn: conn, source: source}
	lockID := int64(728934201)
	if source.Namespace != "" {
		h := fnv.New64a()
		h.Write([]byte(source.Namespace))
		lockID = int64(h.Sum64())
		schema := pgx.Identifier{source.Namespace}.Sanitize()
		if _, err = conn.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+schema+";SET search_path TO "+schema); err != nil {
			conn.Close(ctx)
			return nil, err
		}
	}
	var locked bool
	if err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockID).Scan(&locked); err != nil || !locked {
		conn.Close(ctx)
		return nil, errors.New("PostgreSQL занят другим экземпляром ОМА; остановите его перед запуском")
	}
	if _, err = conn.Exec(ctx, postgresSchema); err != nil {
		conn.Close(ctx)
		return nil, fmt.Errorf("PostgreSQL migration: %w", err)
	}
	return p, nil
}
func (p *Postgres) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	p.conn.Close(ctx)
}
func (p *Postgres) Ping(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.conn.Ping(ctx)
}
func (p *Postgres) Load() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := p.conn.Query(ctx, "SELECT name,payload FROM oma_components ORDER BY name")
	if err != nil {
		return err
	}
	data := map[string]json.RawMessage{}
	for rows.Next() {
		var name string
		var raw []byte
		if err = rows.Scan(&name, &raw); err != nil {
			rows.Close()
			return err
		}
		data[name] = raw
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(data) == 0 {
		// Explicit opt-in migration only; never overwrite an existing database with a file.
		if p.source.ImportNamespace != "" {
			var exists bool
			if err = p.conn.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", p.source.ImportNamespace+".oma_components").Scan(&exists); err != nil {
				return err
			}
			if !exists {
				p.source.ImportNamespace = ""
			}
		}
		if p.source.ImportNamespace != "" {
			table := pgx.Identifier{p.source.ImportNamespace, "oma_components"}.Sanitize()
			for name, c := range p.source.Components {
				var raw []byte
				err := p.conn.QueryRow(ctx, "SELECT payload FROM "+table+" WHERE name=$1", name).Scan(&raw)
				if err == nil {
					if err = c.Import(raw); err != nil {
						return err
					}
				} else if err != pgx.ErrNoRows {
					return err
				}
			}
		}
		if p.source.Path != "" {
			if err = p.source.Load(); err != nil {
				return err
			}
		}
		return p.saveLocked(ctx)
	}
	for name, c := range p.source.Components {
		raw, ok := data[name]
		if !ok {
			return fmt.Errorf("PostgreSQL missing component %s", name)
		}
		if err = c.Import(raw); err != nil {
			return fmt.Errorf("PostgreSQL restore %s: %w", name, err)
		}
	}
	if raw, ok := data["application"]; ok {
		return json.Unmarshal(raw, p.source.Extra)
	}
	return errors.New("PostgreSQL missing application state")
}
func (p *Postgres) Save() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return p.saveLocked(ctx)
}
func (p *Postgres) saveLocked(ctx context.Context) error {
	data := map[string]json.RawMessage{}
	for name, c := range p.source.Components {
		raw, e := c.Export()
		if e != nil {
			return e
		}
		data[name] = raw
	}
	extra, e := json.Marshal(p.source.Extra)
	if e != nil {
		return e
	}
	data["application"] = extra
	tx, e := p.conn.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(context.Background())
	for name, raw := range data {
		if _, e = tx.Exec(ctx, `INSERT INTO oma_components(name,payload) VALUES($1,$2::jsonb) ON CONFLICT(name) DO UPDATE SET payload=excluded.payload,updated_at=now()`, name, string(raw)); e != nil {
			return e
		}
	}
	if _, e = tx.Exec(ctx, "UPDATE oma_runtime SET revision=revision+1,updated_at=now() WHERE singleton=true"); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
