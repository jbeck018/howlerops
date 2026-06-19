package services

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/jbeck018/howlerops/pkg/database"
)

// schemaFakeDatabase implements the schema-introspection surface of
// database.Database over in-memory fixtures. Other methods come from the
// embedded fakeDatabase. Used to drive DatabaseService.GetConnectionSchemaFull.
type schemaFakeDatabase struct {
	*fakeDatabase
	schemas    []string
	tables     map[string][]database.TableInfo     // schema -> tables
	structures map[string]*database.TableStructure // "schema.table" -> structure
	tableErr   map[string]error                    // "schema.table" -> error to return
}

func (s *schemaFakeDatabase) GetSchemas(ctx context.Context) ([]string, error) {
	return s.schemas, nil
}

func (s *schemaFakeDatabase) GetTables(ctx context.Context, schema string) ([]database.TableInfo, error) {
	return s.tables[schema], nil
}

func (s *schemaFakeDatabase) GetTableStructure(ctx context.Context, schema, table string) (*database.TableStructure, error) {
	key := schema + "." + table
	if err := s.tableErr[key]; err != nil {
		return nil, err
	}
	st, ok := s.structures[key]
	if !ok {
		return nil, fmt.Errorf("no structure for %s", key)
	}
	return st, nil
}

func newSchemaFakeDB() *schemaFakeDatabase {
	mkStruct := func(schema, table string, cols ...string) *database.TableStructure {
		columns := make([]database.ColumnInfo, len(cols))
		for i, c := range cols {
			columns[i] = database.ColumnInfo{Name: c, DataType: "text", OrdinalPosition: i + 1}
		}
		return &database.TableStructure{
			Table:   database.TableInfo{Schema: schema, Name: table, Type: "BASE TABLE"},
			Columns: columns,
		}
	}
	return &schemaFakeDatabase{
		fakeDatabase: &fakeDatabase{},
		schemas:      []string{"public", "analytics"},
		tables: map[string][]database.TableInfo{
			"public":    {{Schema: "public", Name: "users"}, {Schema: "public", Name: "orders"}},
			"analytics": {{Schema: "analytics", Name: "events"}},
		},
		structures: map[string]*database.TableStructure{
			"public.users":     mkStruct("public", "users", "id", "email"),
			"public.orders":    mkStruct("public", "orders", "id", "user_id"),
			"analytics.events": mkStruct("analytics", "events", "id", "ts"),
		},
		tableErr: map[string]error{},
	}
}

func newSchemaTestService(db database.Database) *DatabaseService {
	manager := &stubDatabaseManager{
		getConnectionFn: func(connectionID string) (database.Database, error) {
			return db, nil
		},
	}
	service := NewDatabaseServiceWithDependencies(newSilentLogger(), manager, newRecordingEmitter())
	service.SetContext(context.Background())
	return service
}

func TestGetConnectionSchemaFull(t *testing.T) {
	fake := newSchemaFakeDB()
	service := newSchemaTestService(fake)

	schemas, tables, structures, err := service.GetConnectionSchemaFull("conn-1")
	if err != nil {
		t.Fatalf("GetConnectionSchemaFull returned error: %v", err)
	}

	// Schemas preserved in order.
	if len(schemas) != 2 || schemas[0] != "public" || schemas[1] != "analytics" {
		t.Fatalf("unexpected schemas: %v", schemas)
	}

	// Flat table list spans all schemas.
	if len(tables) != 3 {
		t.Fatalf("expected 3 tables, got %d (%v)", len(tables), tables)
	}
	tableNames := make([]string, len(tables))
	for i, tb := range tables {
		tableNames[i] = tb.Schema + "." + tb.Name
	}
	sort.Strings(tableNames)
	want := []string{"analytics.events", "public.orders", "public.users"}
	for i := range want {
		if tableNames[i] != want[i] {
			t.Fatalf("table list mismatch: got %v want %v", tableNames, want)
		}
	}

	// One structure per table, index-aligned and non-nil.
	if len(structures) != 3 {
		t.Fatalf("expected 3 structures, got %d", len(structures))
	}
	byKey := map[string]*database.TableStructure{}
	for _, st := range structures {
		if st == nil {
			t.Fatal("structures contained a nil entry")
		}
		byKey[st.Table.Schema+"."+st.Table.Name] = st
	}
	// Parity with individual GetTableStructure calls.
	for key, want := range fake.structures {
		got, ok := byKey[key]
		if !ok {
			t.Fatalf("missing structure for %s", key)
		}
		if len(got.Columns) != len(want.Columns) {
			t.Fatalf("%s: column count mismatch got %d want %d", key, len(got.Columns), len(want.Columns))
		}
		for i := range want.Columns {
			if got.Columns[i].Name != want.Columns[i].Name {
				t.Fatalf("%s col[%d]: got %s want %s", key, i, got.Columns[i].Name, want.Columns[i].Name)
			}
		}
	}
}

func TestGetConnectionSchemaFullPropagatesTableError(t *testing.T) {
	fake := newSchemaFakeDB()
	fake.tableErr["public.orders"] = fmt.Errorf("boom")
	service := newSchemaTestService(fake)

	_, _, _, err := service.GetConnectionSchemaFull("conn-1")
	if err == nil {
		t.Fatal("expected error from failing table introspection, got nil")
	}
}
