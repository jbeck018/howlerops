package services

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jbeck018/howlerops/pkg/database"
)

// TestGetConnectionSchemaFullE2E_SQLite drives the new batched endpoint through
// the REAL database.Manager against a REAL SQLite database (CGO driver), seeding
// two tables with a foreign key and asserting GetConnectionSchemaFull reflects
// the actual schema. This exercises the full backend path the Wails binding
// invokes: GetSchemas -> GetTables -> concurrent GetTableStructure -> assembly.
func TestGetConnectionSchemaFullE2E_SQLite(t *testing.T) {
	logger := newSilentLogger()
	manager := database.NewManager(logger)
	ctx := context.Background()

	dbPath := filepath.Join(t.TempDir(), fmt.Sprintf("e2e_%d.db", time.Now().UnixNano()))
	cfg := database.ConnectionConfig{
		Type:              database.SQLite,
		Database:          dbPath,
		ConnectionTimeout: 30 * time.Second,
		IdleTimeout:       5 * time.Minute,
		MaxConnections:    25,
		MaxIdleConns:      5,
		Parameters:        map[string]string{"mode": "rwc"},
	}

	conn, err := manager.CreateConnection(ctx, cfg)
	if err != nil {
		t.Fatalf("CreateConnection failed: %v", err)
	}
	defer func() { _ = manager.RemoveConnection(conn.ID) }()

	db, err := manager.GetConnection(conn.ID)
	if err != nil {
		t.Fatalf("GetConnection failed: %v", err)
	}

	// Seed a real schema with a foreign key.
	ddl := []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT NOT NULL)`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, FOREIGN KEY (user_id) REFERENCES users(id))`,
	}
	for _, stmt := range ddl {
		if _, err := db.Execute(ctx, stmt); err != nil {
			t.Fatalf("seed DDL failed (%q): %v", stmt, err)
		}
	}

	service := NewDatabaseServiceWithDependencies(logger, manager, newRecordingEmitter())
	service.SetContext(ctx)

	schemas, tables, structures, err := service.GetConnectionSchemaFull(conn.ID)
	if err != nil {
		t.Fatalf("GetConnectionSchemaFull failed: %v", err)
	}

	// SQLite reports a single "main" schema.
	if len(schemas) != 1 || schemas[0] != "main" {
		t.Fatalf("unexpected schemas: %v", schemas)
	}

	// Both seeded tables are present.
	found := map[string]bool{}
	for _, tb := range tables {
		found[tb.Name] = true
	}
	if !found["users"] || !found["orders"] {
		t.Fatalf("expected users+orders tables, got %v", tables)
	}

	// One structure per table.
	if len(structures) != 2 {
		t.Fatalf("expected 2 structures, got %d", len(structures))
	}
	byName := map[string]*database.TableStructure{}
	for _, st := range structures {
		if st == nil {
			t.Fatal("nil structure entry")
		}
		byName[st.Table.Name] = st
	}

	// users has a primary-key column.
	users := byName["users"]
	if users == nil || len(users.Columns) != 2 {
		t.Fatalf("users structure wrong: %+v", users)
	}
	hasPK := false
	for _, c := range users.Columns {
		if c.PrimaryKey {
			hasPK = true
		}
	}
	if !hasPK {
		t.Fatalf("users missing primary key column: %+v", users.Columns)
	}

	// orders carries the real foreign key referencing users.
	orders := byName["orders"]
	if orders == nil {
		t.Fatal("missing orders structure")
	}
	if len(orders.ForeignKeys) == 0 {
		t.Fatalf("expected a foreign key on orders, got none: %+v", orders)
	}
	fkOK := false
	for _, fk := range orders.ForeignKeys {
		if fk.ReferencedTable == "users" {
			fkOK = true
		}
	}
	if !fkOK {
		t.Fatalf("orders FK does not reference users: %+v", orders.ForeignKeys)
	}
}
