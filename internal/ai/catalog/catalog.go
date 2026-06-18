// Package catalog provides an up-to-date catalog of LLM models sourced from
// models.dev (https://models.dev, MIT licensed). A filtered snapshot of the
// relevant providers is embedded at build time so the app always has a sensible
// list offline, and the live catalog can be fetched at runtime to stay current
// without a new release.
package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// APIURL is the models.dev machine-readable catalog endpoint.
const APIURL = "https://models.dev/api.json"

// Provider is a single provider entry in the catalog.
type Provider struct {
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	Env    []string         `json:"env,omitempty"`
	Models map[string]Model `json:"models"`
}

// Model is the metadata for a single model.
type Model struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Family      string     `json:"family,omitempty"`
	Attachment  bool       `json:"attachment,omitempty"`
	Reasoning   bool       `json:"reasoning,omitempty"`
	ToolCall    bool       `json:"tool_call,omitempty"`
	Temperature bool       `json:"temperature,omitempty"`
	Knowledge   string     `json:"knowledge,omitempty"`
	ReleaseDate string     `json:"release_date,omitempty"`
	LastUpdated string     `json:"last_updated,omitempty"`
	OpenWeights bool       `json:"open_weights,omitempty"`
	Status      string     `json:"status,omitempty"`
	Limit       Limit      `json:"limit"`
	Cost        Cost       `json:"cost"`
	Modalities  Modalities `json:"modalities"`
}

// Limit holds context/output token limits.
type Limit struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

// Cost holds per-million-token pricing.
type Cost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read,omitempty"`
	CacheWrite float64 `json:"cache_write,omitempty"`
}

// Modalities lists supported input/output modalities.
type Modalities struct {
	Input  []string `json:"input,omitempty"`
	Output []string `json:"output,omitempty"`
}

// Catalog is a concurrency-safe collection of providers and their models.
type Catalog struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// Default returns a catalog populated from the embedded snapshot.
func Default() *Catalog {
	c := &Catalog{providers: map[string]Provider{}}
	if err := c.load(snapshot); err != nil {
		// An unparsable embedded snapshot is a build-time error; keep the catalog
		// empty rather than panicking so the app still starts.
		c.providers = map[string]Provider{}
	}
	return c
}

var sharedCatalog = sync.OnceValue(func() *Catalog { return Default() })

// Shared returns a process-wide catalog loaded from the embedded snapshot.
func Shared() *Catalog { return sharedCatalog() }

func (c *Catalog) load(data []byte) error {
	var raw map[string]Provider
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.mu.Lock()
	c.providers = raw
	c.mu.Unlock()
	return nil
}

// Refresh fetches the live catalog from models.dev and replaces the in-memory
// data. It is best-effort: callers should treat an error as "keep the snapshot".
func (c *Catalog) Refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, APIURL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("models.dev: unexpected status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	return c.load(data)
}

// catalogProviderKey maps an app provider name onto a models.dev provider key.
func catalogProviderKey(appProvider string) string {
	switch strings.ToLower(strings.TrimSpace(appProvider)) {
	case "openai", "codex":
		return "openai"
	case "anthropic", "claudecode", "claude":
		return "anthropic"
	case "huggingface":
		return "huggingface"
	case "google", "gemini":
		return "google"
	case "mistral":
		return "mistral"
	case "xai", "grok":
		return "xai"
	case "deepseek":
		return "deepseek"
	case "groq":
		return "groq"
	default:
		return strings.ToLower(strings.TrimSpace(appProvider))
	}
}

// Models returns the catalog models for an app provider, newest first.
func (c *Catalog) Models(appProvider string) []Model {
	key := catalogProviderKey(appProvider)
	c.mu.RLock()
	p, ok := c.providers[key]
	c.mu.RUnlock()
	if !ok {
		return nil
	}
	out := make([]Model, 0, len(p.Models))
	for id, m := range p.Models {
		if m.ID == "" {
			m.ID = id
		}
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].LastUpdated != out[j].LastUpdated {
			return out[i].LastUpdated > out[j].LastUpdated
		}
		return out[i].ReleaseDate > out[j].ReleaseDate
	})
	return out
}

// Lookup returns the catalog metadata for a specific model id under an app
// provider, if present.
func (c *Catalog) Lookup(appProvider, id string) (Model, bool) {
	key := catalogProviderKey(appProvider)
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.providers[key]
	if !ok {
		return Model{}, false
	}
	if m, ok := p.Models[id]; ok {
		if m.ID == "" {
			m.ID = id
		}
		return m, true
	}
	return Model{}, false
}
