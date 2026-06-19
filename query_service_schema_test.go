package main

import (
	"testing"

	"github.com/jbeck018/howlerops/pkg/database"
)

func strPtr(s string) *string { return &s }
func i64Ptr(i int64) *int64   { return &i }
func intPtr(i int) *int       { return &i }

// TestMapTableStructureDTO verifies field parity between the backend
// database.TableStructure and the frontend-facing TableStructure DTO. This is
// the shared mapper used by both GetTableStructure and GetConnectionSchemaFull.
func TestMapTableStructureDTO(t *testing.T) {
	src := &database.TableStructure{
		Table: database.TableInfo{
			Schema:    "public",
			Name:      "users",
			Type:      "BASE TABLE",
			Comment:   "app users",
			RowCount:  42,
			SizeBytes: 8192,
		},
		Columns: []database.ColumnInfo{
			{
				Name:               "id",
				DataType:           "integer",
				Nullable:           false,
				DefaultValue:       strPtr("nextval('users_id_seq')"),
				PrimaryKey:         true,
				Unique:             true,
				Indexed:            true,
				Comment:            "pk",
				OrdinalPosition:    1,
				CharacterMaxLength: nil,
				NumericPrecision:   intPtr(32),
				NumericScale:       intPtr(0),
				Metadata:           map[string]string{"k": "v"},
			},
			{
				Name:               "email",
				DataType:           "varchar",
				Nullable:           true,
				PrimaryKey:         false,
				OrdinalPosition:    2,
				CharacterMaxLength: i64Ptr(255),
			},
		},
		Indexes: []database.IndexInfo{
			{Name: "users_pkey", Columns: []string{"id"}, Unique: true, Primary: true, Type: "btree", Method: "btree"},
		},
		ForeignKeys: []database.ForeignKeyInfo{
			{
				Name:              "users_org_fk",
				Columns:           []string{"org_id"},
				ReferencedTable:   "orgs",
				ReferencedSchema:  "public",
				ReferencedColumns: []string{"id"},
				OnDelete:          "CASCADE",
				OnUpdate:          "NO ACTION",
			},
		},
		Triggers:   []string{"trg_audit"},
		Statistics: map[string]string{"n_live_tup": "42"},
	}

	dto := mapTableStructureDTO(src)

	// Table parity
	if dto.Table.Schema != "public" || dto.Table.Name != "users" || dto.Table.RowCount != 42 || dto.Table.SizeBytes != 8192 {
		t.Fatalf("table parity mismatch: %+v", dto.Table)
	}

	// Columns parity
	if len(dto.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(dto.Columns))
	}
	c0 := dto.Columns[0]
	if c0.Name != "id" || c0.DataType != "integer" || !c0.PrimaryKey || !c0.Unique || !c0.Indexed ||
		c0.OrdinalPosition != 1 || c0.DefaultValue == nil || *c0.DefaultValue != "nextval('users_id_seq')" ||
		c0.NumericPrecision == nil || *c0.NumericPrecision != 32 || c0.Metadata["k"] != "v" {
		t.Fatalf("column[0] parity mismatch: %+v", c0)
	}
	c1 := dto.Columns[1]
	if c1.Name != "email" || !c1.Nullable || c1.CharacterMaxLength == nil || *c1.CharacterMaxLength != 255 {
		t.Fatalf("column[1] parity mismatch: %+v", c1)
	}

	// Index parity
	if len(dto.Indexes) != 1 || dto.Indexes[0].Name != "users_pkey" || !dto.Indexes[0].Primary {
		t.Fatalf("index parity mismatch: %+v", dto.Indexes)
	}

	// Foreign key parity
	if len(dto.ForeignKeys) != 1 {
		t.Fatalf("expected 1 fk, got %d", len(dto.ForeignKeys))
	}
	fk := dto.ForeignKeys[0]
	if fk.Name != "users_org_fk" || fk.ReferencedTable != "orgs" || fk.ReferencedSchema != "public" ||
		len(fk.Columns) != 1 || fk.Columns[0] != "org_id" || len(fk.ReferencedColumns) != 1 ||
		fk.ReferencedColumns[0] != "id" || fk.OnDelete != "CASCADE" {
		t.Fatalf("fk parity mismatch: %+v", fk)
	}

	// Triggers + statistics passthrough
	if len(dto.Triggers) != 1 || dto.Triggers[0] != "trg_audit" || dto.Statistics["n_live_tup"] != "42" {
		t.Fatalf("triggers/statistics parity mismatch: triggers=%v stats=%v", dto.Triggers, dto.Statistics)
	}
}
