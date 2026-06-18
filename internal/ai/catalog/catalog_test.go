package catalog

import "testing"

func TestEmbeddedSnapshotLoads(t *testing.T) {
	c := Default()
	if got := c.Models("anthropic"); len(got) == 0 {
		t.Fatal("expected anthropic models from embedded snapshot, got none")
	}
	if got := c.Models("openai"); len(got) == 0 {
		t.Fatal("expected openai models from embedded snapshot, got none")
	}
}

func TestCurrentClaudeAliasesPresent(t *testing.T) {
	c := Default()
	for _, id := range []string{"claude-opus-4-8", "claude-sonnet-4-6", "claude-haiku-4-5"} {
		if _, ok := c.Lookup("anthropic", id); !ok {
			t.Errorf("expected current Claude alias %q in catalog", id)
		}
	}
}

func TestProviderAliasMapping(t *testing.T) {
	c := Default()
	// codex maps onto the openai catalog
	if got := c.Models("codex"); len(got) == 0 {
		t.Fatal("expected codex to resolve to openai catalog models")
	}
	// claudecode maps onto the anthropic catalog
	if _, ok := c.Lookup("claudecode", "claude-opus-4-8"); !ok {
		t.Error("expected claudecode to resolve to anthropic catalog")
	}
}

func TestModelsNewestFirst(t *testing.T) {
	c := Default()
	models := c.Models("anthropic")
	if len(models) < 2 {
		t.Skip("not enough models to check ordering")
	}
	if models[0].LastUpdated < models[len(models)-1].LastUpdated {
		t.Errorf("expected newest-first ordering, first=%s last=%s",
			models[0].LastUpdated, models[len(models)-1].LastUpdated)
	}
}
