package core

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/gob"
	"errors"
	"fmt"
	"os"

	"ragflow/internal/common"
	"ragflow/internal/harness/core/schema"

	"go.uber.org/zap"
)

// ---- Resume types ----

type ResumeInfo struct {
	EnableStreaming bool
	*InterruptInfo
	WasInterrupted bool
	InterruptState any
	IsResumeTarget bool
	ResumeData     any
}

type InterruptInfo struct {
	Data              any
	InterruptContexts []*InterruptCtx
}

// ---- Address types ----

type Address = []AddressSegment

type AddressSegment struct {
	Type AddressSegmentType
	ID   string
}

type AddressSegmentType string

const (
	AddressSegmentAgent AddressSegmentType = "agent"
	AddressSegmentTool  AddressSegmentType = "tool"
)

type InterruptCtx struct {
	ID      string
	Address Address
	Info    any
	State   any
}

type InterruptSignal struct {
	ID       string
	Address  Address
	Info     any
	State    any
	Children []*InterruptSignal
}

// ---- Interrupt constructors ----

func Interrupt(ctx context.Context, info any) *AgentEvent {
	return TypedInterrupt[*schema.Message](ctx, info)
}

func TypedInterrupt[M MessageType](ctx context.Context, info any) *TypedAgentEvent[M] {
	return &TypedAgentEvent[M]{Action: &AgentAction{Interrupted: &InterruptInfo{Data: info}}}
}

func StatefulInterrupt(ctx context.Context, info, state any) *AgentEvent {
	return TypedStatefulInterrupt[*schema.Message](ctx, info, state)
}

func TypedStatefulInterrupt[M MessageType](ctx context.Context, info, state any) *TypedAgentEvent[M] {
	addr := captureAddress(ctx)
	return &TypedAgentEvent[M]{Action: &AgentAction{
		Interrupted: &InterruptInfo{Data: info},
		internalInterrupted: &InterruptSignal{
			Info: info, State: state, Address: addr,
		},
	}}
}

func CompositeInterrupt(ctx context.Context, info, state any, subs ...*InterruptSignal) *AgentEvent {
	return TypedCompositeInterrupt[*schema.Message](ctx, info, state, subs...)
}

func TypedCompositeInterrupt[M MessageType](ctx context.Context, info, state any, subs ...*InterruptSignal) *TypedAgentEvent[M] {
	addr := captureAddress(ctx)
	children := make([]*InterruptSignal, len(subs))
	for i, sub := range subs {
		cp := *sub
		children[i] = &cp
	}
	return &TypedAgentEvent[M]{Action: &AgentAction{
		Interrupted: &InterruptInfo{Data: info},
		internalInterrupted: &InterruptSignal{
			Info: info, State: state, Address: addr, Children: children,
		},
	}}
}

// captureAddress copies the current address segments from context.
func captureAddress(ctx context.Context) Address {
	segs := getAddressSegments(ctx)
	if len(segs) == 0 {
		return nil
	}
	addr := make(Address, len(segs))
	copy(addr, segs)
	return addr
}

type addrSegKey struct{}

func AppendAddressSegment(ctx context.Context, t AddressSegmentType, id string) context.Context {
	parent, _ := ctx.Value(addrSegKey{}).([]AddressSegment)
	seg := make([]AddressSegment, len(parent)+1)
	copy(seg, parent)
	seg[len(parent)] = AddressSegment{Type: t, ID: id}
	return context.WithValue(ctx, addrSegKey{}, seg)
}

func getAddressSegments(ctx context.Context) []AddressSegment {
	if v, ok := ctx.Value(addrSegKey{}).([]AddressSegment); ok {
		return v
	}
	return nil
}

// FromInterruptContexts builds an InterruptSignal tree from a flat slice of
// InterruptCtx. Returns nil when ctxs is empty.
func FromInterruptContexts(ctxs []*InterruptCtx) *InterruptSignal {
	if len(ctxs) == 0 {
		return nil
	}
	root := &InterruptSignal{}
	buildFromCtxs(ctxs, root)
	return root
}

func buildFromCtxs(ctxs []*InterruptCtx, parent *InterruptSignal) {
	for _, c := range ctxs {
		sig := &InterruptSignal{
			ID: c.ID, Address: make(Address, len(c.Address)),
			Info: c.Info, State: c.State,
		}
		copy(sig.Address, c.Address)
		parent.Children = append(parent.Children, sig)
	}
}

// ---- Checkpoint store ----

type CheckPointStore interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, data []byte) error
}

// InterruptState wraps the opaque interrupt state for checkpoint serialization.
// Callers MUST register the concrete type stored in State via schema.RegisterName
// or gob.Register before saving a checkpoint; otherwise gob.Encode/Decode will
// panic at runtime for unregistered interface types.
type InterruptState struct{ State any }

type checkpointPayload struct {
	RunCtx              *runContext
	Info                *InterruptInfo
	EnableStreaming     bool
	InterruptID2Address map[string]Address
	InterruptID2State   map[string]InterruptState
	TenantID            string
}

func init() {
	schema.RegisterType("agentcore_checkpoint", func() any { return &checkpointPayload{} })
	schema.RegisterType("agentcore_interrupt_state", func() any { return &InterruptState{} })
}

// ---- Checkpoint tenant isolation ----

type checkpointTenantKey struct{}

const DefaultCheckpointTenantKey = "tenant_id"

// WithCheckpointTenant embeds a tenant ID in the context for checkpoint tenant isolation.
// loadCheckpoint will reject checkpoints whose TenantID does not match this value.
func WithCheckpointTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, checkpointTenantKey{}, tenantID)
}

func extractCheckpointTenant(ctx context.Context) string {
	if tid, ok := ctx.Value(checkpointTenantKey{}).(string); ok && tid != "" {
		return tid
	}
	if rc := getRunCtx(ctx); rc != nil && rc.Session != nil {
		if tid, ok := rc.Session.Values[DefaultCheckpointTenantKey].(string); ok {
			return tid
		}
	}
	return ""
}

// ---- Checkpoint integrity (HMAC) ----

const (
	hmacLen    = 32
	envHMACKey = "CHECKPOINT_HMAC_KEY"
)

// checkpointHMACKey reads the HMAC key from the CHECKPOINT_HMAC_KEY env var
// (base64-encoded, 32 bytes). If unset, a random key is generated per startup
// with a log warning — this is safe for single-process in-memory usage but
// will BREAK checkpoint resume across process restarts. Production deployments
// MUST set CHECKPOINT_HMAC_KEY to a stable base64-encoded 32-byte secret.
var checkpointHMACKey = loadCheckpointHMACKey()

func loadCheckpointHMACKey() []byte {
	if env := os.Getenv(envHMACKey); env != "" {
		k, err := base64.StdEncoding.DecodeString(env)
		if err != nil {
			panic("checkpoint HMAC key: invalid base64 in " + envHMACKey + ": " + err.Error())
		}
		if len(k) != 32 {
			panic("checkpoint HMAC key: " + envHMACKey + " must decode to exactly 32 bytes, got " + fmt.Sprintf("%d", len(k)))
		}
		return k
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		panic("failed to generate checkpoint HMAC key: " + err.Error())
	}
	common.Warn("checkpoint HMAC env not set — using random per-process key; checkpoint resume across restarts will fail", zap.String("env", envHMACKey))
	return k
}

func computeCheckpointHMAC(payload []byte) []byte {
	mac := hmac.New(sha256.New, checkpointHMACKey)
	mac.Write(payload)
	return mac.Sum(nil)
}

func loadCheckpoint(store CheckPointStore, ctx context.Context, cid string) (context.Context, *runContext, *ResumeInfo, error) {
	data, exist, err := store.Get(ctx, cid)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("checkpoint get: %w", err)
	}
	if !exist {
		return nil, nil, nil, fmt.Errorf("checkpoint %s not found", cid)
	}

	// Split: first 32 bytes = HMAC, rest = payload
	if len(data) < hmacLen {
		return nil, nil, nil, fmt.Errorf("checkpoint %s too short (%d bytes)", cid, len(data))
	}
	mac, payload := data[:hmacLen], data[hmacLen:]

	// Verify HMAC
	expected := computeCheckpointHMAC(payload)
	if !hmac.Equal(mac, expected) {
		return nil, nil, nil, fmt.Errorf("checkpoint %s integrity check failed", cid)
	}

	var p checkpointPayload
	if err := gob.NewDecoder(bytes.NewReader(payload)).Decode(&p); err != nil {
		return nil, nil, nil, fmt.Errorf("decode checkpoint: %w", err)
	}

	// Verify tenant isolation
	// Policy: when EITHER side carries a TenantID, BOTH must be present and match.
	// Empty-on-both-sides is allowed for backward compat (non-tenant deployments).
	currentTenant := extractCheckpointTenant(ctx)
	if p.TenantID != "" || currentTenant != "" {
		if p.TenantID == "" {
			return nil, nil, nil, fmt.Errorf("checkpoint %s tenant mismatch: stored is empty, current=%q", cid, currentTenant)
		}
		if currentTenant == "" {
			return nil, nil, nil, fmt.Errorf("checkpoint %s tenant mismatch: stored=%q, current is empty", cid, p.TenantID)
		}
		if p.TenantID != currentTenant {
			return nil, nil, nil, fmt.Errorf("checkpoint %s tenant mismatch: stored=%q current=%q", cid, p.TenantID, currentTenant)
		}
	}

	// Rebuild InterruptContexts from checkpoint maps
	ics := mapsToInterruptContexts(p.InterruptID2Address, p.InterruptID2State)
	if p.Info != nil {
		p.Info.InterruptContexts = ics
	}

	return ctx, p.RunCtx, &ResumeInfo{
		EnableStreaming: p.EnableStreaming,
		InterruptInfo:   p.Info,
	}, nil
}

func saveCheckpoint(store CheckPointStore, ctx context.Context, key string, enableStreaming bool, info *InterruptInfo, is *InterruptSignal) error {
	if store == nil {
		return nil
	}
	rc := getRunCtx(ctx)
	id2addr, id2state := signalToMaps(is)
	tenantID := extractCheckpointTenant(ctx)

	// Encode payload with tenant ID
	p := checkpointPayload{
		RunCtx: rc, Info: info, EnableStreaming: enableStreaming,
		InterruptID2Address: id2addr, InterruptID2State: id2state,
		TenantID: tenantID,
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(p); err != nil {
		return fmt.Errorf("encode checkpoint: %w", err)
	}
	payload := buf.Bytes()

	// Prepend HMAC for integrity verification
	mac := computeCheckpointHMAC(payload)
	stored := make([]byte, 0, hmacLen+len(payload))
	stored = append(stored, mac...)
	stored = append(stored, payload...)

	return store.Set(ctx, key, stored)
}

// signalToMaps recursively walks the InterruptSignal tree (is.Children) to build
// flat ID-to-Address and ID-to-State maps for checkpoint serialization.
// Children are populated by buildFromCtxs (called from FromInterruptContexts) or
// by CompositeInterrupt/TypedCompositeInterrupt constructors.
func signalToMaps(is *InterruptSignal) (map[string]Address, map[string]InterruptState) {
	a, s := make(map[string]Address), make(map[string]InterruptState)
	if is == nil {
		return a, s
	}
	if is.ID != "" {
		a[is.ID] = is.Address
		if is.State != nil {
			s[is.ID] = InterruptState{State: is.State}
		}
	}
	for _, c := range is.Children {
		ca, cs := signalToMaps(c)
		for k, v := range ca {
			a[k] = v
		}
		for k, v := range cs {
			s[k] = v
		}
	}
	return a, s
}

// mapsToInterruptContexts reconstructs a slice of InterruptCtx from checkpoint maps.
func mapsToInterruptContexts(id2addr map[string]Address, id2state map[string]InterruptState) []*InterruptCtx {
	if len(id2addr) == 0 {
		return nil
	}
	ics := make([]*InterruptCtx, 0, len(id2addr))
	for id, addr := range id2addr {
		ic := &InterruptCtx{ID: id, Address: make(Address, len(addr))}
		copy(ic.Address, addr)
		if st, ok := id2state[id]; ok {
			ic.State = st.State
		}
		ics = append(ics, ic)
	}
	return ics
}

// getNextResumeAgent returns the deepest (innermost) agent address segment for
// single-agent resume routing. It scans address segments from the end.
func getNextResumeAgent(ctx context.Context, info *ResumeInfo) (string, error) {
	segs := getAddressSegments(ctx)
	if len(segs) == 0 {
		return "", errors.New("no address segments for resume")
	}
	// Find the deepest agent segment
	for i := len(segs) - 1; i >= 0; i-- {
		if segs[i].Type == AddressSegmentAgent {
			return segs[i].ID, nil
		}
	}
	return "", errors.New("no agent address segment found for resume")
}

// getNextResumeAgents returns ALL agent address segments for multi-agent resume
// routing (e.g., parallel branches). Returns all agent segments as a set.
func getNextResumeAgents(ctx context.Context, info *ResumeInfo) (map[string]bool, error) {
	segs := getAddressSegments(ctx)
	if len(segs) == 0 {
		return nil, errors.New("no address segments for resume")
	}
	result := make(map[string]bool)
	for _, s := range segs {
		if s.Type == AddressSegmentAgent {
			result[s.ID] = true
		}
	}
	if len(result) == 0 {
		return nil, errors.New("no agent address segments found for resume")
	}
	return result, nil
}

// buildResumeInfo copies all ResumeInfo fields into a new struct and appends
// the agent address segment. IsResumeTarget and ResumeData are always copied
// regardless of WasInterrupted — callers that set them for non-interrupted
// resumes (e.g., initial resume of a fresh run) should have them preserved.
func buildResumeInfo(ctx context.Context, nextID string, info *ResumeInfo) (context.Context, *ResumeInfo) {
	ctx = AppendAddressSegment(ctx, AddressSegmentAgent, nextID)
	ri := &ResumeInfo{
		EnableStreaming: info.EnableStreaming,
		InterruptInfo:   info.InterruptInfo,
		WasInterrupted:  info.WasInterrupted,
		IsResumeTarget:  info.IsResumeTarget,
		ResumeData:      info.ResumeData,
	}
	ctx = updateRunPathOnly(ctx, nextID)
	return ctx, ri
}
