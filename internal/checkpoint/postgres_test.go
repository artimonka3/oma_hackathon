package checkpoint

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

type testPart struct {
	Value string
	Fail  bool
}

func (p *testPart) Export() (json.RawMessage, error) {
	if p.Fail {
		return json.RawMessage(`[]`), nil
	}
	return json.Marshal(map[string]string{"value": p.Value})
}
func (p *testPart) Import(raw json.RawMessage) error {
	var v map[string]string
	if e := json.Unmarshal(raw, &v); e != nil {
		return e
	}
	p.Value = v["value"]
	return nil
}
func TestPostgresCommitRestoreAndRollback(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable database ending in _test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	part := &testPart{Value: "first"}
	extra := map[string]string{"project": "preserved"}
	source := &Store{Components: map[string]Component{"test": part}, Extra: &extra}
	db, e := OpenPostgres(ctx, url, source)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var name string
	if e = db.conn.QueryRow(ctx, "SELECT current_database()").Scan(&name); e != nil || !strings.HasSuffix(name, "_test") {
		t.Fatal("test requires a disposable *_test database")
	}
	if _, e = db.conn.Exec(ctx, "TRUNCATE oma_components"); e != nil {
		t.Fatal(e)
	}
	if e = db.Load(); e != nil {
		t.Fatal(e)
	}
	part.Value = "committed"
	if e = db.Save(); e != nil {
		t.Fatal(e)
	}
	second, e := OpenPostgres(ctx, url, source)
	if e == nil {
		second.Close()
		t.Fatal("second writer was allowed")
	}
	part.Value = "discard"
	part.Fail = true
	if e = db.Save(); e == nil {
		t.Fatal("invalid JSON shape must roll back transaction")
	}
	part.Fail = false
	// Restore using a fresh connection and freshly constructed domain state.
	db.Close()
	db, e = OpenPostgres(ctx, url, source)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = db.Load(); e != nil {
		t.Fatal(e)
	}
	if part.Value != "committed" || extra["project"] != "preserved" {
		t.Fatal("committed data not restored")
	}
}
