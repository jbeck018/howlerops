package multiquery

import "testing"

func TestFederationRewriter_Rewrite(t *testing.T) {
	aliases := map[string]string{
		"proda": "db_aaa",
		"prodb": "db_bbb",
	}
	r := NewFederationRewriter(aliases)

	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{
			name: "schema-qualified across two connections",
			in:   "SELECT * FROM @prodA.public.users u JOIN @prodB.public.orders o ON u.id=o.user_id",
			want: `SELECT * FROM "db_aaa".public.users u JOIN "db_bbb".public.orders o ON u.id=o.user_id`,
		},
		{
			name: "table-only (no schema)",
			in:   "SELECT * FROM @prodA.users",
			want: `SELECT * FROM "db_aaa".users`,
		},
		{
			name: "case-insensitive connection name",
			in:   "SELECT * FROM @PRODA.users",
			want: `SELECT * FROM "db_aaa".users`,
		},
		{
			name: "literal containing @ is untouched",
			in:   "SELECT * FROM @prodA.users WHERE email LIKE '%@prodB.com%'",
			want: `SELECT * FROM "db_aaa".users WHERE email LIKE '%@prodB.com%'`,
		},
		{
			name:    "unknown connection errors",
			in:      "SELECT * FROM @unknown.users",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.Rewrite(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (out=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("rewrite mismatch:\n in:   %s\n got:  %s\n want: %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestDuckAliasForSession_Deterministic(t *testing.T) {
	a := DuckAliasForSession("session-123")
	b := DuckAliasForSession("session-123")
	c := DuckAliasForSession("session-456")
	if a != b {
		t.Fatalf("alias not deterministic: %s != %s", a, b)
	}
	if a == c {
		t.Fatalf("aliases for different sessions collided: %s", a)
	}
	if len(a) == 0 || a[:3] != "db_" {
		t.Fatalf("unexpected alias form: %s", a)
	}
}
