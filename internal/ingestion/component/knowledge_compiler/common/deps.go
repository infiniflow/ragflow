package common

import (
	"context"
	"fmt"
	"sync"
)

// DefaultLLMContextLength is the fallback chat-model context window (tokens)
// used for per-cluster text truncation when the model's real max length is
// unknown (e.g. in tests). Mirrors Python's default llm_model.max_length.
const DefaultLLMContextLength = 8192

// ChatRequest is the minimal surface a variant needs to dispatch one LLM call.
type ChatRequest struct {
	LLMID        string
	SystemPrompt string
	UserPrompt   string
	JSONMode     bool
	// Temperature is optional; nil leaves the driver default. Python's
	// knowledge compilation pins extraction at 0.1 and merge judging at 0.0,
	// so variants set it per call site.
	Temperature *float64
	// MaxTokens caps the generated summary length (mirrors Python's
	// {"max_tokens": max(self._max_token, 512)} config, issue #10235).
	MaxTokens *int
	APIKey    string
	BaseURL   string
}

// ChatResponse holds the LLM's text answer.
type ChatResponse struct {
	Content string
}

// ChatInvoker dispatches a single LLM chat call. The production path routes
// through internal/entity/models (same factory as other ingestion components);
// tests inject a canned responder.
type ChatInvoker interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}

// Embedder wraps a text embedding model. Encode returns one vector per input.
type Embedder interface {
	Encode(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
}

// Tokenizer estimates token counts for budget packing.
type Tokenizer interface {
	NumTokens(text string) int
}

// HistoricalHit is one cross-run KNN candidate returned by HistoricalKNN.
type HistoricalHit struct {
	ID    string
	Score float64
}

// HistoricalKNN is the read-only ES KNN candidate lookup used by the wiki
// variant for cross-run dedup. nil in M1; wired in M5/M9.
type HistoricalKNN interface {
	TopKHistory(ctx context.Context, tenantID, datasetID, variant string, vec []float32, k int, threshold float64) ([]HistoricalHit, error)
}

// WikiPageCandidate is one existing wiki page returned from the backing store.
type WikiPageCandidate struct {
	ID             string
	Slug           string
	Title          string
	PageType       string
	Topic          string
	Summary        string
	ContentMD      string
	ContentMDRaw   string
	EntityNames    []string
	RelatedKBPages []string
	Outlinks       []string
	Score          float64
}

// WikiPageStore is the read-only storage seam the wiki variant uses to
// reconcile against existing artifact_page rows and to load the current page
// body for UPDATE merges.
type WikiPageStore interface {
	FindSimilarPages(ctx context.Context, tenantID, datasetID string, queryVec []float32, k int) ([]WikiPageCandidate, error)
	GetPageBySlug(ctx context.Context, tenantID, datasetID, slug string) (*WikiPageCandidate, error)
}

// RedisClient is injected only for the datasetnav variant (M8).
type RedisClient interface {
	// Minimal lock surface; full API added in M8.
}

// Deps bundles the runtime capabilities a variant needs. All fields are
// interfaces so tests inject stubs; production wiring lives in
// internal/ingestion/task (see PORT_PLAN.md §4 dependency injection seam).
type Deps struct {
	Chat          ChatInvoker
	Embed         Embedder
	Tokenizer     Tokenizer
	HistoricalKNN HistoricalKNN // optional (wiki)
	WikiPages     WikiPageStore // optional (wiki)
	Redis         RedisClient   // optional (datasetnav)
	TenantID      string
	DatasetID     string
	// LLMMaxLength is the chat model's context window in tokens. RAPTOR uses it
	// to truncate each cluster's texts so the summary prompt fits the window
	// (mirrors Python self._llm_model.max_length).
	LLMMaxLength int
}

// DepsResolver resolves the per-run Deps from a tenant/llm/embedding triple.
type DepsResolver func(tenantID, llmID, embeddingModel string) (Deps, error)

// DepsResolver / SetDepsResolver / ResolveDeps are the only injection seams the
// component exposes. The component is DB-independent: it compiles knowledge
// units into chunk-aligned docs (see conf/infinity_mapping.json) and returns
// them merged into the upstream chunk stream, so it owns the product schema but
// not the storage engine. Persistence (if any) is the caller's concern.

var (
	depsResolverMu sync.RWMutex
	depsResolver   DepsResolver
)

// SetDepsResolver installs the production (or test) DepsResolver. Tests call
// this to inject stubs; production wiring calls it from an init() in
// internal/ingestion/task.
func SetDepsResolver(r DepsResolver) {
	depsResolverMu.Lock()
	defer depsResolverMu.Unlock()
	depsResolver = r
}

// ResolveDeps resolves Deps via the installed resolver.
func ResolveDeps(tenantID, llmID, embeddingModel string) (Deps, error) {
	depsResolverMu.RLock()
	r := depsResolver
	depsResolverMu.RUnlock()
	if r == nil {
		return Deps{}, fmt.Errorf("knowledge_compiler: no DepsResolver registered (call common.SetDepsResolver in production wiring)")
	}
	return r(tenantID, llmID, embeddingModel)
}

// GroupResolver resolves compilation-template-group ids to the concrete
// compilation_template ids they contain. It is a DB-backed seam installed by
// production wiring in internal/ingestion/task; the knowledge_compiler package
// itself stays DB-independent. When a component is configured with
// compilation_template_group_ids but no GroupResolver is installed, the
// component fails loudly instead of silently emitting rows that miss
// compilation_template_ids (a data-loss path).
type GroupResolver func(ctx context.Context, tenantID string, groupIDs []string) ([]string, error)

var (
	groupResolverMu sync.RWMutex
	groupResolver   GroupResolver
)

// SetGroupResolver installs the production (or test) GroupResolver. Production
// wiring calls this from an init() in internal/ingestion/task; tests may inject
// a stub.
func SetGroupResolver(r GroupResolver) {
	groupResolverMu.Lock()
	defer groupResolverMu.Unlock()
	groupResolver = r
}

// ResolveGroupTemplateIDs resolves the given group ids to template ids via the
// installed resolver. Returns (nil, nil) when there are no group ids to
// resolve. Returns an error if group ids are requested but no resolver is
// installed, surfacing the misconfiguration instead of silently dropping the
// compilation_template_ids stamp.
func ResolveGroupTemplateIDs(ctx context.Context, tenantID string, groupIDs []string) ([]string, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	groupResolverMu.RLock()
	r := groupResolver
	groupResolverMu.RUnlock()
	if r == nil {
		return nil, fmt.Errorf("knowledge_compiler: compilation_template_group_ids provided but no GroupResolver installed (production wiring must call common.SetGroupResolver)")
	}
	return r(ctx, tenantID, groupIDs)
}
