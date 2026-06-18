package multiquery

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

// DuckAliasForSession derives a deterministic, valid DuckDB catalog alias for a
// connection sessionId. It is stable across queries (so attachments can be
// cached and reused) and collision-free across distinct sessions. The hash is
// used only to produce a short, valid identifier — it is not security-sensitive.
func DuckAliasForSession(sessionID string) string {
	sum := sha256.Sum256([]byte(sessionID))
	return "db_" + hex.EncodeToString(sum[:])[:12]
}

// federationRewritePattern matches a single/double-quoted literal (which must be
// left untouched so an '@' inside a string isn't treated as a reference) OR an
// @conn[.schema].table reference.
var federationRewritePattern = regexp.MustCompile(
	`'(?:[^']|'')*'|"(?:[^"]|"")*"|@([\w-]+)((?:\.[\w-]+){1,2})`)

// FederationRewriter rewrites @conn[.schema].table references into
// alias[.schema].table, where alias is the DuckDB catalog the connection was
// attached under.
type FederationRewriter struct {
	aliasFor map[string]string // lowercased connection name -> duckdb alias
}

// NewFederationRewriter builds a rewriter from a map of connection name -> duck
// alias (as returned by the federation backend). Lookups are case-insensitive,
// matching @conn resolution elsewhere.
func NewFederationRewriter(aliasByName map[string]string) *FederationRewriter {
	lower := make(map[string]string, len(aliasByName))
	for name, alias := range aliasByName {
		lower[strings.ToLower(name)] = alias
	}
	return &FederationRewriter{aliasFor: lower}
}

// Rewrite replaces @conn references with their attached DuckDB alias, leaving
// string/identifier literals untouched. Returns an error if a referenced
// connection has no mapping.
func (r *FederationRewriter) Rewrite(sql string) (string, error) {
	var missing string
	out := federationRewritePattern.ReplaceAllStringFunc(sql, func(match string) string {
		if len(match) == 0 || match[0] != '@' {
			return match // a string/identifier literal — leave as-is
		}
		sub := federationRewritePattern.FindStringSubmatch(match)
		connName := sub[1]
		rest := sub[2] // ".schema.table" or ".table"
		alias, ok := r.aliasFor[strings.ToLower(connName)]
		if !ok {
			if missing == "" {
				missing = connName
			}
			return match
		}
		return `"` + alias + `"` + rest
	})
	if missing != "" {
		return "", fmt.Errorf("no attached database for @%s", missing)
	}
	return out, nil
}
