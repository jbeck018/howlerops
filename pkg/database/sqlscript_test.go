package database

import (
	"reflect"
	"testing"
)

func TestSplitSQLStatements(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   []string
	}{
		{
			name:   "single statement no semicolon",
			script: "SELECT 1",
			want:   []string{"SELECT 1"},
		},
		{
			name:   "single statement trailing semicolon",
			script: "SELECT 1;",
			want:   []string{"SELECT 1"},
		},
		{
			name:   "two statements",
			script: "SELECT 1; SELECT 2;",
			want:   []string{"SELECT 1", "SELECT 2"},
		},
		{
			name:   "transaction block",
			script: "BEGIN;\nINSERT INTO t VALUES (1);\nCOMMIT;",
			want:   []string{"BEGIN", "INSERT INTO t VALUES (1)", "COMMIT"},
		},
		{
			name:   "semicolon inside single-quoted literal",
			script: "INSERT INTO t (msg) VALUES ('a; b; c'); SELECT 1;",
			want:   []string{"INSERT INTO t (msg) VALUES ('a; b; c')", "SELECT 1"},
		},
		{
			name:   "escaped single quote inside literal",
			script: "SELECT 'Let''s reconnect; really'; SELECT 2;",
			want:   []string{"SELECT 'Let''s reconnect; really'", "SELECT 2"},
		},
		{
			name:   "semicolon inside quoted identifier",
			script: `SELECT * FROM "weird;name"; SELECT 2;`,
			want:   []string{`SELECT * FROM "weird;name"`, "SELECT 2"},
		},
		{
			name:   "semicolon inside dollar quoted string",
			script: "DO $$ BEGIN PERFORM 1; PERFORM 2; END $$; SELECT 9;",
			want:   []string{"DO $$ BEGIN PERFORM 1; PERFORM 2; END $$", "SELECT 9"},
		},
		{
			name:   "tagged dollar quote",
			script: "SELECT $body$ a;b $body$; SELECT 2;",
			want:   []string{"SELECT $body$ a;b $body$", "SELECT 2"},
		},
		{
			name:   "semicolon inside line comment",
			script: "SELECT 1 -- a; b\n; SELECT 2;",
			want:   []string{"SELECT 1 -- a; b", "SELECT 2"},
		},
		{
			name:   "semicolon inside block comment",
			script: "SELECT 1 /* a; b */; SELECT 2;",
			want:   []string{"SELECT 1 /* a; b */", "SELECT 2"},
		},
		{
			name:   "comment-only tail is dropped",
			script: "SELECT 1;\n-- just a trailing comment\n",
			want:   []string{"SELECT 1"},
		},
		{
			name:   "blank statements between semicolons dropped",
			script: "SELECT 1;;; SELECT 2;",
			want:   []string{"SELECT 1", "SELECT 2"},
		},
		{
			name:   "dollar parameter placeholders are not dollar quotes",
			script: "SELECT $1; SELECT $2;",
			want:   []string{"SELECT $1", "SELECT $2"},
		},
		{
			name:   "empty script",
			script: "   \n  ",
			want:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitSQLStatements(tt.script)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("splitSQLStatements(%q)\n  got:  %#v\n  want: %#v", tt.script, got, tt.want)
			}
		})
	}
}

func TestStatementReturnsRows(t *testing.T) {
	tests := []struct {
		stmt string
		want bool
	}{
		{"SELECT 1", true},
		{"select 1", true},
		{"  \n SELECT 1", true},
		{"-- lead comment\nSELECT 1", true},
		{"/* c */ WITH x AS (SELECT 1) SELECT * FROM x", true},
		{"VALUES (1), (2)", true},
		{"TABLE foo", true},
		{"SHOW search_path", true},
		{"EXPLAIN SELECT 1", true},
		{"CALL do_thing()", true},
		{"INSERT INTO t (a) VALUES (1) RETURNING id", true},
		{"UPDATE t SET a = 1 WHERE id = 2 RETURNING *", true},
		{"BEGIN", false},
		{"COMMIT", false},
		{"INSERT INTO t (a) VALUES (1)", false},
		{"UPDATE t SET a = 1", false},
		{"DELETE FROM t WHERE id = 1", false},
		{"CREATE TABLE t (id int)", false},
		{"SET LOCAL statement_timeout = '5s'", false},
		// "returning" only inside a string literal must not count.
		{"INSERT INTO t (msg) VALUES ('returning home')", false},
	}

	for _, tt := range tests {
		t.Run(tt.stmt, func(t *testing.T) {
			if got := statementReturnsRows(tt.stmt); got != tt.want {
				t.Fatalf("statementReturnsRows(%q) = %v, want %v", tt.stmt, got, tt.want)
			}
		})
	}
}

func TestLeadingKeyword(t *testing.T) {
	tests := []struct {
		stmt string
		want string
	}{
		{"SELECT 1", "SELECT"},
		{"  with x as (select 1) select * from x", "WITH"},
		{"-- comment\n  insert into t values (1)", "INSERT"},
		{"/* block */ BEGIN", "BEGIN"},
		{"123", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.stmt, func(t *testing.T) {
			if got := leadingKeyword(tt.stmt); got != tt.want {
				t.Fatalf("leadingKeyword(%q) = %q, want %q", tt.stmt, got, tt.want)
			}
		})
	}
}
