package services

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	cron "github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"

	"github.com/jbeck018/howlerops/internal/reportbind"
	"github.com/jbeck018/howlerops/internal/runner"
	"github.com/jbeck018/howlerops/pkg/ai"
	"github.com/jbeck018/howlerops/pkg/alerts"
	"github.com/jbeck018/howlerops/pkg/database"
	"github.com/jbeck018/howlerops/pkg/export"
	"github.com/jbeck018/howlerops/pkg/materialization"
	"github.com/jbeck018/howlerops/pkg/storage"
)

// ReportService coordinates report persistence and execution.
type ReportService struct {
	storage      *storage.ReportStorage
	db           *DatabaseService
	ai           *ai.Service
	exporter     *export.Exporter
	alertEngine  *alerts.AlertEngine
	materializer *materialization.Materializer
	logger       *logrus.Logger
	ctx          context.Context
	mu           sync.RWMutex
	cron         *cron.Cron
	entries      map[string]cron.EntryID
	cache        *queryCache
	workerLimit  int // max concurrent component executions
}

// ReportRunRequest represents a run invocation from the UI.
type ReportRunRequest struct {
	ReportID     string                 `json:"reportId"`
	ComponentIDs []string               `json:"componentIds"`
	FilterValues map[string]interface{} `json:"filters"`
	Force        bool                   `json:"force"`
}

// ReportRunResponse contains aggregated component data.
type ReportRunResponse struct {
	ReportID    string                  `json:"reportId"`
	StartedAt   time.Time               `json:"startedAt"`
	CompletedAt time.Time               `json:"completedAt"`
	Results     []ReportComponentResult `json:"results"`
}

// ReportComponentResult wraps query or LLM outputs.
type ReportComponentResult struct {
	ComponentID string                      `json:"componentId"`
	Type        storage.ReportComponentType `json:"type"`
	Columns     []string                    `json:"columns,omitempty"`
	Rows        [][]interface{}             `json:"rows,omitempty"`
	RowCount    int64                       `json:"rowCount,omitempty"`
	DurationMS  int64                       `json:"durationMs,omitempty"`
	Content     string                      `json:"content,omitempty"`
	Metadata    map[string]any              `json:"metadata,omitempty"`
	Error       string                      `json:"error,omitempty"`
	CacheHit    bool                        `json:"cacheHit,omitempty"`
	TotalRows   int64                       `json:"totalRows,omitempty"`   // Total before LIMIT
	LimitedRows int                         `json:"limitedRows,omitempty"` // Actual rows returned
}

// NewReportService builds a new report service.
func NewReportService(logger *logrus.Logger, db *DatabaseService) *ReportService {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	cronScheduler := cron.New(cron.WithParser(parser), cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)))
	cronScheduler.Start()

	// Initialize export service (doesn't need DB)
	exporter := export.NewExporter(logger)

	return &ReportService{
		logger:      logger,
		db:          db,
		exporter:    exporter,
		cron:        cronScheduler,
		entries:     make(map[string]cron.EntryID),
		cache:       newQueryCache(100 * 1024 * 1024), // 100MB cache
		workerLimit: 5,                                // max 5 concurrent components
	}
}

// SetContext wires the Wails context for downstream calls.
func (s *ReportService) SetContext(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ctx = ctx
}

// SetStorage injects the report storage implementation once available.
func (s *ReportService) SetStorage(storage *storage.ReportStorage) {
	s.mu.Lock()
	s.storage = storage
	s.mu.Unlock()

	go s.bootstrapSchedules()
}

// SetAlertEngine injects the alert engine implementation.
func (s *ReportService) SetAlertEngine(engine *alerts.AlertEngine) {
	s.mu.Lock()
	s.alertEngine = engine
	s.mu.Unlock()

	if engine != nil {
		engine.SetEvaluateComponentCallback(s.evaluateComponentForAlert)
	}
}

// SetMaterializer injects the materialization service.
func (s *ReportService) SetMaterializer(materializer *materialization.Materializer) {
	s.mu.Lock()
	s.materializer = materializer
	s.mu.Unlock()

	if materializer != nil {
		materializer.SetRunReportCallback(s.runReportForMaterialization)
	}
}

// SetAIService wires the AI runtime for LLM blocks.
func (s *ReportService) SetAIService(aiService *ai.Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ai = aiService
}

// Shutdown stops the background scheduler.
func (s *ReportService) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron != nil {
		s.cron.Stop()
		s.cron = nil
	}
	s.entries = make(map[string]cron.EntryID)
}

func (s *ReportService) withStorage() (*storage.ReportStorage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.storage == nil {
		return nil, fmt.Errorf("report storage not initialised")
	}
	return s.storage, nil
}

func (s *ReportService) withAI() *ai.Service {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ai
}

func (s *ReportService) bootstrapSchedules() {
	store, err := s.withStorage()
	if err != nil {
		return
	}
	reports, err := store.ListReports()
	if err != nil {
		s.logger.WithError(err).Warn("Failed to list reports for scheduling")
		return
	}

	for _, summary := range reports {
		report, err := store.GetReport(summary.ID)
		if err != nil {
			s.logger.WithError(err).WithField("report_id", summary.ID).Warn("Failed to load report for scheduling")
			continue
		}
		s.configureSchedule(report)
	}
}

func (s *ReportService) ensureCronLocked() {
	if s.cron == nil {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		s.cron = cron.New(cron.WithParser(parser), cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)))
		s.cron.Start()
	}
	if s.entries == nil {
		s.entries = make(map[string]cron.EntryID)
	}
}

func (s *ReportService) configureSchedule(report *storage.Report) {
	if report == nil || report.ID == "" {
		return
	}

	// Skip local scheduling when the report is configured for a remote target
	if strings.EqualFold(strings.TrimSpace(report.SyncOptions.Target), "remote") {
		s.removeSchedule(report.ID)
		return
	}

	cadence := strings.TrimSpace(report.SyncOptions.Cadence)
	if !report.SyncOptions.Enabled || cadence == "" {
		s.removeSchedule(report.ID)
		return
	}

	normalized := normalizeCadence(cadence)
	reportID := report.ID

	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCronLocked()

	if entry, ok := s.entries[reportID]; ok {
		s.cron.Remove(entry)
		delete(s.entries, reportID)
	}

	entryID, err := s.cron.AddFunc(normalized, func() {
		if _, err := s.RunReport(&ReportRunRequest{ReportID: reportID}); err != nil {
			s.logger.WithError(err).WithField("report_id", reportID).Warn("Scheduled report execution failed")
		}
	})
	if err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"report_id": reportID,
			"cadence":   cadence,
		}).Warn("Failed to schedule report")
		return
	}

	s.entries[reportID] = entryID
}

func (s *ReportService) removeSchedule(reportID string) {
	if reportID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron == nil {
		return
	}
	if entry, ok := s.entries[reportID]; ok {
		s.cron.Remove(entry)
		delete(s.entries, reportID)
	}
}

// SaveReport persists a report definition.
func (s *ReportService) SaveReport(report *storage.Report) (*storage.Report, error) {
	store, err := s.withStorage()
	if err != nil {
		return nil, err
	}

	if err := store.SaveReport(report); err != nil {
		return nil, err
	}

	s.configureSchedule(report)

	return report, nil
}

// ListReports returns report summaries for navigation lists.
func (s *ReportService) ListReports() ([]storage.ReportSummary, error) {
	store, err := s.withStorage()
	if err != nil {
		return nil, err
	}
	return store.ListReports()
}

// GetReport returns a specific report definition.
func (s *ReportService) GetReport(id string) (*storage.Report, error) {
	store, err := s.withStorage()
	if err != nil {
		return nil, err
	}
	return store.GetReport(id)
}

// DeleteReport removes a report definition.
func (s *ReportService) DeleteReport(id string) error {
	store, err := s.withStorage()
	if err != nil {
		return err
	}
	if err := store.DeleteReport(id); err != nil {
		return err
	}
	s.removeSchedule(id)
	return nil
}

// RunReport executes configured components in parallel with worker pool.
func (s *ReportService) RunReport(req *ReportRunRequest) (*ReportRunResponse, error) {
	store, err := s.withStorage()
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}
	if req.ReportID == "" {
		return nil, fmt.Errorf("report id is required")
	}

	report, err := store.GetReport(req.ReportID)
	if err != nil {
		return nil, err
	}

	// Clear cache if force flag is set
	if req.Force {
		s.cache.clear()
	}

	// Build component filter
	componentFilter := map[string]struct{}{}
	for _, id := range req.ComponentIDs {
		componentFilter[id] = struct{}{}
	}

	started := time.Now()

	// Filter components to execute
	componentsToRun := make([]*storage.ReportComponent, 0)
	for i := range report.Definition.Components {
		component := &report.Definition.Components[i]
		if len(componentFilter) > 0 {
			if _, ok := componentFilter[component.ID]; !ok {
				continue
			}
		}
		componentsToRun = append(componentsToRun, component)
	}

	// Execute components in parallel
	results := s.runComponentsParallel(report, componentsToRun, req.FilterValues)

	resp := &ReportRunResponse{
		ReportID:    report.ID,
		StartedAt:   started,
		CompletedAt: time.Now(),
		Results:     results,
	}

	// Check for errors
	status := "ok"
	var firstErr error
	for _, res := range results {
		if res.Error != "" && firstErr == nil {
			firstErr = fmt.Errorf("%s", res.Error)
			status = "error"
		}
	}

	if err := store.UpdateRunState(report.ID, status, resp.CompletedAt); err != nil {
		s.logger.WithError(err).Warn("Failed to update report run state")
	}

	// Log cache statistics
	if s.cache != nil {
		stats := s.cache.stats()
		s.logger.WithFields(logrus.Fields{
			"report_id":       report.ID,
			"cache_entries":   stats["entries"],
			"cache_hits":      stats["totalHits"],
			"cache_util_pct":  fmt.Sprintf("%.1f%%", stats["utilization"].(float64)*100),
			"total_duration":  time.Since(started),
			"component_count": len(results),
		}).Debug("Report execution completed")
	}

	if firstErr != nil {
		return resp, firstErr
	}

	return resp, nil
}

// componentTask represents a component execution task
type componentTask struct {
	component *storage.ReportComponent
	index     int // original index for ordering
	filters   map[string]interface{}
}

// runComponentsParallel executes query components concurrently, then LLM
// components. Query components have no inter-dependencies, so they run in a
// single worker-pool phase. LLM components may reference prior query output, so
// they execute in a second phase against one immutable snapshot of the results —
// avoiding the per-task index rebuild and guaranteeing their context is complete.
func (s *ReportService) runComponentsParallel(
	report *storage.Report,
	components []*storage.ReportComponent,
	filters map[string]interface{},
) []ReportComponentResult {
	results := make([]ReportComponentResult, len(components))
	if len(components) == 0 {
		return results
	}

	// Partition components into query vs LLM, preserving original indexes.
	var queryTasks, llmTasks []componentTask
	for i, component := range components {
		task := componentTask{component: component, index: i, filters: filters}
		if component.Type == storage.ReportComponentLLM {
			llmTasks = append(llmTasks, task)
		} else {
			queryTasks = append(queryTasks, task)
		}
	}

	// Phase 1: query components (no shared dependencies) run via the shared
	// runner engine, bounded by the worker limit. Each component is a step with
	// no dependencies; the runner schedules them concurrently.
	if len(queryTasks) > 0 {
		querySteps := make([]runner.Step, 0, len(queryTasks))
		for _, task := range queryTasks {
			task := task
			querySteps = append(querySteps, runner.Step{
				ID: task.component.ID,
				Run: func(ctx context.Context, _ map[string]runner.Result) (any, error) {
					return s.runComponentWithTimeout(ctx, report, task.component, task.filters, nil), nil
				},
			})
		}
		runResults, runErr := runner.Run(context.Background(), runner.Plan{Steps: querySteps}, runner.Options{
			MaxParallel:    s.workerLimit,
			DefaultTimeout: 5 * time.Minute,
		})
		if runErr != nil {
			// Only an invalid plan (e.g. duplicate component IDs) reaches here;
			// record the error against each query component rather than dropping
			// results silently.
			s.logger.WithError(runErr).Error("report runner plan invalid")
			for _, task := range queryTasks {
				results[task.index] = ReportComponentResult{
					ComponentID: task.component.ID,
					Type:        task.component.Type,
					Error:       runErr.Error(),
				}
			}
		} else {
			for _, task := range queryTasks {
				if r, ok := runResults[task.component.ID]; ok {
					if rc, ok := r.Output.(ReportComponentResult); ok {
						results[task.index] = rc
					}
				}
			}
		}
	}

	if len(llmTasks) == 0 {
		return results
	}

	// Phase 2: build a snapshot of completed query results, then run LLM
	// components sequentially in their original order, folding each result back
	// into the snapshot. This keeps LLM->query context deterministic AND lets an
	// LLM component reference an earlier LLM component's output (the old single
	// unordered pool satisfied LLM->LLM only by lucky scheduling).
	prior := make(map[string]ReportComponentResult, len(results))
	for _, res := range results {
		if res.ComponentID != "" {
			prior[res.ComponentID] = res
		}
	}

	for _, task := range llmTasks {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		res := s.runComponentWithTimeout(ctx, report, task.component, task.filters, prior)
		cancel()
		results[task.index] = res
		if res.ComponentID != "" {
			prior[res.ComponentID] = res
		}
	}

	return results
}

// runComponentWithTimeout executes a component with panic recovery
func (s *ReportService) runComponentWithTimeout(
	ctx context.Context,
	report *storage.Report,
	component *storage.ReportComponent,
	filters map[string]interface{},
	resultIndex map[string]ReportComponentResult,
) (result ReportComponentResult) {
	// Recover from panics
	defer func() {
		if r := recover(); r != nil {
			s.logger.WithFields(logrus.Fields{
				"component_id": component.ID,
				"panic":        r,
			}).Error("Component execution panicked")
			result = ReportComponentResult{
				ComponentID: component.ID,
				Type:        component.Type,
				Error:       fmt.Sprintf("Internal error: %v", r),
			}
		}
	}()

	// Check context cancellation
	select {
	case <-ctx.Done():
		return ReportComponentResult{
			ComponentID: component.ID,
			Type:        component.Type,
			Error:       "Execution timeout exceeded",
		}
	default:
	}

	return s.runComponent(report, component, filters, resultIndex)
}

func (s *ReportService) runComponent(report *storage.Report, component *storage.ReportComponent, filters map[string]interface{}, cache map[string]ReportComponentResult) ReportComponentResult {
	switch component.Type {
	case storage.ReportComponentLLM:
		return s.runLLMComponent(report, component, filters, cache)
	default:
		return s.runQueryComponent(report, component, filters)
	}
}

func (s *ReportService) runQueryComponent(report *storage.Report, component *storage.ReportComponent, filters map[string]interface{}) ReportComponentResult {
	res := ReportComponentResult{
		ComponentID: component.ID,
		Type:        component.Type,
	}

	if s.db == nil {
		res.Error = "database service not available"
		return res
	}

	queryConfig := component.Query
	if queryConfig.ConnectionID == "" {
		res.Error = "component missing connection"
		return res
	}

	sqlText := strings.TrimSpace(queryConfig.SQL)
	if sqlText == "" {
		res.Error = "component SQL is empty"
		return res
	}

	boundSQL, bindErr := applyFilterPlaceholders(sqlText, report, component, filters)
	if bindErr != nil {
		res.Error = fmt.Sprintf("parameter binding failed: %v", bindErr)
		return res
	}
	sqlText = boundSQL

	// Check cache first (if TTL is configured and > 0)
	cacheTTL := time.Duration(queryConfig.CacheSeconds) * time.Second
	if cacheTTL > 0 && s.cache != nil {
		cacheKey := s.cache.cacheKey(queryConfig.ConnectionID, sqlText, filters)
		if cached, hit := s.cache.get(cacheKey); hit {
			s.logger.WithFields(logrus.Fields{
				"component_id": component.ID,
				"cache_key":    cacheKey[:16], // log partial key
			}).Debug("Cache hit for query component")
			return *cached
		}
	}

	// Determine limit with safety bounds
	const (
		defaultLimit = 1000
		maxLimit     = 50000
	)

	limit := defaultLimit
	if queryConfig.Limit != nil && *queryConfig.Limit > 0 {
		limit = *queryConfig.Limit
		if limit > maxLimit {
			res.Error = fmt.Sprintf(
				"Query limit %d exceeds maximum allowed limit of %d. Please reduce the limit in component settings.",
				limit, maxLimit,
			)
			return res
		}
	}

	// Execute the query once. The database layer applies the LIMIT and, in the
	// same call, reports the full TotalRows count and HasMore flag, so a separate
	// SELECT COUNT(*) round-trip is unnecessary. (Most drivers can't batch a
	// count + select into a single statement anyway, and an unbounded count
	// re-scans the entire result set — doubling latency for local execution.)
	opts := &database.QueryOptions{
		Timeout:  60 * time.Second,
		ReadOnly: true,
		Limit:    limit,
	}

	started := time.Now()
	result, err := s.db.ExecuteQuery(queryConfig.ConnectionID, sqlText, opts)
	if err != nil {
		res.Error = err.Error()
		res.DurationMS = int64(time.Since(started).Milliseconds())
		return res
	}
	res.DurationMS = int64(time.Since(started).Milliseconds())

	// Reject result sets that exceed the configured limit, using the total count
	// the driver already computed (no extra query). A zero total means the driver
	// could not determine the count, in which case we return the limited rows.
	totalRows := result.TotalRows
	if totalRows > int64(limit) {
		res.Error = fmt.Sprintf(
			"Query returned %d rows but limit is %d. Please add WHERE clause or increase limit in component settings (max %d).",
			totalRows, limit, maxLimit,
		)
		res.TotalRows = totalRows
		res.LimitedRows = 0
		return res
	}

	res.Columns = result.Columns
	res.Rows = result.Rows
	res.RowCount = result.RowCount
	res.TotalRows = totalRows
	if totalRows > int64(len(result.Rows)) {
		res.LimitedRows = len(result.Rows)
	}

	// Cache result if TTL is configured
	if cacheTTL > 0 && s.cache != nil {
		cacheKey := s.cache.cacheKey(queryConfig.ConnectionID, sqlText, filters)
		s.cache.set(cacheKey, res, cacheTTL)
		s.logger.WithFields(logrus.Fields{
			"component_id": component.ID,
			"cache_key":    cacheKey[:16],
			"ttl_seconds":  queryConfig.CacheSeconds,
		}).Debug("Cached query result")
	}

	return res
}

func (s *ReportService) runLLMComponent(report *storage.Report, component *storage.ReportComponent, filters map[string]interface{}, prior map[string]ReportComponentResult) ReportComponentResult {
	res := ReportComponentResult{
		ComponentID: component.ID,
		Type:        component.Type,
	}

	aiService := s.withAI()
	if aiService == nil {
		res.Error = "AI service not configured"
		return res
	}
	if component.LLM == nil {
		res.Error = "LLM settings missing"
		return res
	}

	contextBlob := buildContextPayload(component.LLM.ContextComponents, prior)
	prompt := injectPlaceholders(component.LLM.PromptTemplate, filters, contextBlob)

	chatReq := &ai.ChatRequest{
		Prompt:      prompt,
		Context:     contextBlob["context"],
		Provider:    component.LLM.Provider,
		Model:       component.LLM.Model,
		MaxTokens:   component.LLM.MaxTokens,
		Temperature: component.LLM.Temperature,
		Metadata: map[string]string{
			"report_id":    report.ID,
			"component_id": component.ID,
		},
	}
	for k, v := range component.LLM.Metadata {
		if chatReq.Metadata == nil {
			chatReq.Metadata = map[string]string{}
		}
		chatReq.Metadata[k] = v
	}

	start := time.Now()
	resp, err := aiService.Chat(s.ctx, chatReq)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Content = resp.Content
	res.Metadata = map[string]any{
		"provider":   resp.Provider,
		"model":      resp.Model,
		"tokens":     resp.TokensUsed,
		"durationMs": time.Since(start).Milliseconds(),
	}

	return res
}

// applyFilterPlaceholders delegates to the reportbind package, which binds the
// component's top-level {{filter}} placeholders to SQL-safe literals via the
// canonical params engine. Kept as a thin wrapper so the call site reads
// clearly and the binding logic stays in a Wails-free, unit-tested package.
func applyFilterPlaceholders(sql string, report *storage.Report, component *storage.ReportComponent, filters map[string]interface{}) (string, error) {
	return reportbind.Apply(sql, report, component, filters)
}

func buildContextPayload(componentIDs []string, prior map[string]ReportComponentResult) map[string]string {
	payload := map[string]string{}
	if len(componentIDs) == 0 {
		return payload
	}

	var builder strings.Builder
	for _, id := range componentIDs {
		result, ok := prior[id]
		if !ok {
			continue
		}
		fragment := map[string]interface{}{
			"componentId": id,
			"columns":     result.Columns,
			"rows":        result.Rows,
		}
		blob, err := json.Marshal(fragment)
		if err != nil {
			continue
		}
		payload["component."+id] = string(blob)
		builder.Write(blob)
		builder.WriteString("\n")
	}

	if builder.Len() > 0 {
		payload["context"] = builder.String()
	}

	return payload
}

func injectPlaceholders(template string, filters map[string]interface{}, context map[string]string) string {
	replacements := map[string]string{}
	for key, value := range filters {
		replacements["{{"+key+"}}"] = fmt.Sprintf("%v", value)
	}
	for key, value := range context {
		replacements["{{"+key+"}}"] = value
	}
	return replaceTokens(template, replacements)
}

func replaceTokens(input string, replacements map[string]string) string {
	if len(replacements) == 0 {
		return input
	}
	// Deterministic ordering for tests/debugging
	keys := make([]string, 0, len(replacements))
	for k := range replacements {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	result := input
	for _, key := range keys {
		result = strings.ReplaceAll(result, key, replacements[key])
	}
	return result
}

func normalizeCadence(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "@every 5m"
	}
	if strings.HasPrefix(trimmed, "@every") {
		return trimmed
	}
	if _, err := time.ParseDuration(trimmed); err == nil {
		return "@every " + trimmed
	}
	return trimmed
}

// ==================== Query Cache Implementation ====================

// cacheEntry represents a cached query result
type cacheEntry struct {
	result    ReportComponentResult
	cachedAt  time.Time
	expiresAt time.Time
	hitCount  int
	size      int64 // estimated memory size in bytes
}

// queryCache provides thread-safe LRU caching for query results
type queryCache struct {
	mu           sync.RWMutex
	entries      map[string]*cacheEntry
	maxSizeBytes int64
	currentSize  int64
}

// newQueryCache creates a new query cache with the specified max size
func newQueryCache(maxSizeBytes int64) *queryCache {
	return &queryCache{
		entries:      make(map[string]*cacheEntry),
		maxSizeBytes: maxSizeBytes,
	}
}

// cacheKey generates a unique key for a query
func (c *queryCache) cacheKey(connectionID, sql string, filters map[string]interface{}) string {
	// Create deterministic key from connection + SQL + filters
	h := sha256.New()
	h.Write([]byte(connectionID))
	h.Write([]byte(sql))

	// Sort filter keys for deterministic hashing
	if len(filters) > 0 {
		keys := make([]string, 0, len(filters))
		for k := range filters {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			h.Write([]byte(k))
			filterJSON, _ := json.Marshal(filters[k])
			h.Write(filterJSON)
		}
	}

	return fmt.Sprintf("%x", h.Sum(nil))
}

// get retrieves a cached result if it exists and hasn't expired
func (c *queryCache) get(key string) (*ReportComponentResult, bool) {
	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists {
		return nil, false
	}

	// Check expiration
	if time.Now().After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.currentSize -= entry.size
		c.mu.Unlock()
		return nil, false
	}

	// Update hit count
	c.mu.Lock()
	entry.hitCount++
	c.mu.Unlock()

	// Return a copy of the result with cache hit flag
	result := entry.result
	result.CacheHit = true
	return &result, true
}

// set stores a result in the cache with the specified TTL
func (c *queryCache) set(key string, result ReportComponentResult, ttl time.Duration) {
	// Estimate size (rough approximation)
	size := int64(len(result.ComponentID) + len(result.Content))
	for _, col := range result.Columns {
		size += int64(len(col))
	}
	for _, row := range result.Rows {
		for _, cell := range row {
			// Rough estimate: 8 bytes per cell + string content
			size += 8
			if str, ok := cell.(string); ok {
				size += int64(len(str))
			}
		}
	}

	entry := &cacheEntry{
		result:    result,
		cachedAt:  time.Now(),
		expiresAt: time.Now().Add(ttl),
		hitCount:  0,
		size:      size,
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict old entry if exists
	if old, exists := c.entries[key]; exists {
		c.currentSize -= old.size
	}

	// Evict LRU entries if we exceed size limit
	for c.currentSize+size > c.maxSizeBytes && len(c.entries) > 0 {
		c.evictLRU()
	}

	c.entries[key] = entry
	c.currentSize += size
}

// evictLRU removes the least recently used (oldest) entry
func (c *queryCache) evictLRU() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.entries {
		if oldestKey == "" || entry.cachedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.cachedAt
		}
	}

	if oldestKey != "" {
		c.currentSize -= c.entries[oldestKey].size
		delete(c.entries, oldestKey)
	}
}

// clear removes all entries from the cache
func (c *queryCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*cacheEntry)
	c.currentSize = 0
}

// stats returns cache statistics
func (c *queryCache) stats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	totalHits := 0
	for _, entry := range c.entries {
		totalHits += entry.hitCount
	}

	return map[string]interface{}{
		"entries":     len(c.entries),
		"sizeBytes":   c.currentSize,
		"maxBytes":    c.maxSizeBytes,
		"utilization": float64(c.currentSize) / float64(c.maxSizeBytes),
		"totalHits":   totalHits,
	}
}

// ==================== Public Cache Management Methods ====================

// ClearCache clears all cached query results
func (s *ReportService) ClearCache() {
	if s.cache != nil {
		s.cache.clear()
		s.logger.Info("Report query cache cleared")
	}
}

// GetCacheStats returns cache statistics
func (s *ReportService) GetCacheStats() map[string]interface{} {
	if s.cache == nil {
		return map[string]interface{}{
			"enabled": false,
		}
	}

	stats := s.cache.stats()
	stats["enabled"] = true
	return stats
}

// ==================== Export Methods ====================

// ExportReport exports report results to CSV, Excel, or PDF
func (s *ReportService) ExportReport(reportID string, format export.ExportFormat, componentIDs []string, filters map[string]interface{}, options export.ExportOptions) (*export.ExportResult, error) {
	store, err := s.withStorage()
	if err != nil {
		return nil, err
	}

	report, err := store.GetReport(reportID)
	if err != nil {
		return nil, err
	}

	// Run report to get results
	req := &ReportRunRequest{
		ReportID:     reportID,
		ComponentIDs: componentIDs,
		FilterValues: filters,
	}

	resp, err := s.RunReport(req)
	if err != nil {
		return nil, fmt.Errorf("failed to run report: %w", err)
	}

	// Convert results to export format
	exportResults := make([]export.ReportComponentResult, len(resp.Results))
	for i, r := range resp.Results {
		exportResults[i] = export.ReportComponentResult{
			ComponentID: r.ComponentID,
			Type:        r.Type,
			Columns:     r.Columns,
			Rows:        r.Rows,
			RowCount:    r.RowCount,
			DurationMS:  r.DurationMS,
			Content:     r.Content,
			Metadata:    r.Metadata,
			Error:       r.Error,
			CacheHit:    r.CacheHit,
			TotalRows:   r.TotalRows,
			LimitedRows: r.LimitedRows,
		}
	}

	return s.exporter.ExportWithFormat(format, report, exportResults, options)
}

// ==================== Alert Methods ====================

// SaveAlertRule creates or updates an alert rule
func (s *ReportService) SaveAlertRule(rule *alerts.AlertRule) error {
	if s.alertEngine == nil {
		return fmt.Errorf("alert engine not initialized")
	}
	return s.alertEngine.SaveRule(rule)
}

// GetAlertRule retrieves an alert rule by ID
func (s *ReportService) GetAlertRule(ruleID string) (*alerts.AlertRule, error) {
	if s.alertEngine == nil {
		return nil, fmt.Errorf("alert engine not initialized")
	}
	return s.alertEngine.GetRule(ruleID)
}

// ListAlertRules lists all alert rules for a report
func (s *ReportService) ListAlertRules(reportID string) ([]*alerts.AlertRule, error) {
	if s.alertEngine == nil {
		return nil, fmt.Errorf("alert engine not initialized")
	}
	return s.alertEngine.ListRulesByReport(reportID)
}

// DeleteAlertRule removes an alert rule
func (s *ReportService) DeleteAlertRule(ruleID string) error {
	if s.alertEngine == nil {
		return fmt.Errorf("alert engine not initialized")
	}
	return s.alertEngine.DeleteRule(ruleID)
}

// TestAlert evaluates an alert rule without persisting results
func (s *ReportService) TestAlert(ruleID string) (*alerts.AlertResult, error) {
	if s.alertEngine == nil {
		return nil, fmt.Errorf("alert engine not initialized")
	}

	rule, err := s.alertEngine.GetRule(ruleID)
	if err != nil {
		return nil, err
	}

	return s.alertEngine.EvaluateAlert(rule)
}

// GetAlertHistory retrieves alert history
func (s *ReportService) GetAlertHistory(ruleID string, limit int) ([]*alerts.AlertHistory, error) {
	if s.alertEngine == nil {
		return nil, fmt.Errorf("alert engine not initialized")
	}
	return s.alertEngine.GetAlertHistory(ruleID, limit)
}

// evaluateComponentForAlert is a callback for the alert engine
func (s *ReportService) evaluateComponentForAlert(reportID, componentID string, filters map[string]interface{}) (*alerts.ComponentResult, error) {
	store, err := s.withStorage()
	if err != nil {
		return nil, err
	}

	report, err := store.GetReport(reportID)
	if err != nil {
		return nil, err
	}

	// Find the component
	var component *storage.ReportComponent
	for i := range report.Definition.Components {
		if report.Definition.Components[i].ID == componentID {
			component = &report.Definition.Components[i]
			break
		}
	}

	if component == nil {
		return nil, fmt.Errorf("component not found: %s", componentID)
	}

	// Run the component
	result := s.runQueryComponent(report, component, filters)

	return &alerts.ComponentResult{
		ComponentID: result.ComponentID,
		Type:        result.Type,
		Columns:     result.Columns,
		Rows:        result.Rows,
		RowCount:    result.RowCount,
		Error:       result.Error,
	}, nil
}

// ==================== Materialization Methods ====================

// MaterializeReport creates a snapshot of report results
func (s *ReportService) MaterializeReport(reportID string, filters map[string]interface{}, ttl time.Duration) (*materialization.Snapshot, error) {
	if s.materializer == nil {
		return nil, fmt.Errorf("materializer not initialized")
	}
	return s.materializer.MaterializeReport(reportID, filters, ttl)
}

// GetReportWithCache retrieves report results, using cache if available
func (s *ReportService) GetReportWithCache(reportID string, filters map[string]interface{}, maxAge time.Duration) (*ReportRunResponse, error) {
	if s.materializer == nil {
		// Fall back to regular execution
		return s.RunReport(&ReportRunRequest{
			ReportID:     reportID,
			FilterValues: filters,
		})
	}

	// Try to get from snapshot
	snapshot, err := s.materializer.GetLatestSnapshot(reportID, filters, maxAge)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to get snapshot, falling back to execution")
	}

	if snapshot != nil {
		s.logger.WithField("snapshot_id", snapshot.Snapshot.ID).Debug("Using materialized snapshot")

		// Convert to response format
		results := make([]ReportComponentResult, len(snapshot.Results))
		for i, r := range snapshot.Results {
			results[i] = ReportComponentResult{
				ComponentID: r.ComponentID,
				Type:        r.Type,
				Columns:     r.Columns,
				Rows:        r.Rows,
				RowCount:    r.RowCount,
				DurationMS:  r.DurationMS,
				Content:     r.Content,
				Metadata:    r.Metadata,
				Error:       r.Error,
				CacheHit:    true, // Mark as cache hit
				TotalRows:   r.TotalRows,
				LimitedRows: r.LimitedRows,
			}
		}

		return &ReportRunResponse{
			ReportID:    reportID,
			StartedAt:   snapshot.Snapshot.CreatedAt,
			CompletedAt: snapshot.Snapshot.CreatedAt,
			Results:     results,
		}, nil
	}

	// No fresh snapshot, run report
	return s.RunReport(&ReportRunRequest{
		ReportID:     reportID,
		FilterValues: filters,
	})
}

// InvalidateCache invalidates materialized snapshots for a report
func (s *ReportService) InvalidateCache(reportID string) error {
	if s.materializer == nil {
		return fmt.Errorf("materializer not initialized")
	}
	return s.materializer.InvalidateSnapshots(reportID)
}

// ListSnapshots lists materialized snapshots for a report
func (s *ReportService) ListSnapshots(reportID string, limit int) ([]*materialization.Snapshot, error) {
	if s.materializer == nil {
		return nil, fmt.Errorf("materializer not initialized")
	}
	return s.materializer.ListSnapshots(reportID, limit)
}

// ScheduleMaterialization schedules periodic materialization
func (s *ReportService) ScheduleMaterialization(reportID, schedule string, filters map[string]interface{}, ttl time.Duration) error {
	if s.materializer == nil {
		return fmt.Errorf("materializer not initialized")
	}
	return s.materializer.ScheduleMaterialization(reportID, schedule, filters, ttl)
}

// runReportForMaterialization is a callback for the materializer
func (s *ReportService) runReportForMaterialization(reportID string, filters map[string]interface{}) ([]materialization.ReportComponentResult, error) {
	resp, err := s.RunReport(&ReportRunRequest{
		ReportID:     reportID,
		FilterValues: filters,
	})
	if err != nil {
		return nil, err
	}

	// Convert to materialization format
	results := make([]materialization.ReportComponentResult, len(resp.Results))
	for i, r := range resp.Results {
		results[i] = materialization.ReportComponentResult{
			ComponentID: r.ComponentID,
			Type:        r.Type,
			Columns:     r.Columns,
			Rows:        r.Rows,
			RowCount:    r.RowCount,
			DurationMS:  r.DurationMS,
			Content:     r.Content,
			Metadata:    r.Metadata,
			Error:       r.Error,
			CacheHit:    r.CacheHit,
			TotalRows:   r.TotalRows,
			LimitedRows: r.LimitedRows,
		}
	}

	return results, nil
}
