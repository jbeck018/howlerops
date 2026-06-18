package database

import (
	"strings"
	"testing"
)

func TestBuildAttachString(t *testing.T) {
	pg := ConnectionConfig{Type: PostgreSQL, Host: "h", Port: 5432, Database: "app", Username: "u", Password: "secret", SSLMode: "require"}
	cs, typ, fp, err := BuildAttachString(pg)
	if err != nil {
		t.Fatalf("postgres: %v", err)
	}
	if typ != "postgres" {
		t.Fatalf("postgres type = %s", typ)
	}
	for _, want := range []string{"host=h", "port=5432", "dbname=app", "user=u", "password=secret", "sslmode=require"} {
		if !strings.Contains(cs, want) {
			t.Fatalf("postgres connStr missing %q: %s", want, cs)
		}
	}
	if fp == "" {
		t.Fatal("expected a fingerprint")
	}

	my := ConnectionConfig{Type: MySQL, Host: "h", Port: 3306, Database: "app", Username: "u", Password: "p"}
	cs, typ, _, err = BuildAttachString(my)
	if err != nil {
		t.Fatalf("mysql: %v", err)
	}
	if typ != "mysql" {
		t.Fatalf("mysql type = %s", typ)
	}
	// Must be libmysql keyword form, NOT the go-sql-driver user:pass@tcp(...) DSN.
	if strings.Contains(cs, "@tcp(") || !strings.Contains(cs, "database=app") {
		t.Fatalf("mysql connStr wrong form: %s", cs)
	}

	lite := ConnectionConfig{Type: SQLite, Database: "/tmp/a.db"}
	cs, typ, _, err = BuildAttachString(lite)
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	if typ != "sqlite" || cs != "/tmp/a.db" {
		t.Fatalf("sqlite connStr/type wrong: %s / %s", cs, typ)
	}

	// Tunneled connections are not yet supported and must error (caller falls back).
	if _, _, _, err := BuildAttachString(ConnectionConfig{Type: PostgreSQL, UseTunnel: true}); err == nil {
		t.Fatal("expected error for tunneled connection")
	}
	// Unsupported engine type must error.
	if _, _, _, err := BuildAttachString(ConnectionConfig{Type: MongoDB}); err == nil {
		t.Fatal("expected error for unsupported engine")
	}
}

func TestAttachFingerprint_ChangesWithCredentials(t *testing.T) {
	base := ConnectionConfig{Type: PostgreSQL, Host: "h", Port: 5432, Database: "app", Username: "u", Password: "p1"}
	changed := base
	changed.Password = "p2"
	if attachFingerprint(base) == attachFingerprint(changed) {
		t.Fatal("fingerprint should change when the password changes")
	}
}
