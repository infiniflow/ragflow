package checkpoint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/harness/graph/constants"

	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

// NATSSaver implements BaseCheckpointer using NATS KV Store (JetStream-backed).
//
// Design:
//   - Single NATS KV bucket shared by all tenants and graph instances.
//   - Key format: "{tenant_id}:{graph_instance_id}" — each graph instance has its own key.
//   - A "graph instance" corresponds to one thread of execution in the Pregel engine.
//
// Garbage Collection (two layers):
//
//	Layer 1 — Per-key version management (zero-touch):
//	  NATS KV's History=N automatically discards old versions per key.
//	  Each graph instance keeps only the latest N checkpoints.
//	  This handles the normal case: an active graph continuously writes,
//	  older versions are naturally evicted by NATS.
//
//	Layer 2 — Completed graph instance cleanup (background):
//	  When a graph finishes execution (or crashes), its key becomes dormant.
//	  The background GC periodically scans all keys and purges those
//	  whose latest checkpoint is older than MaxGraphIdle.
//	  An idle key = a completed/abandoned graph instance.
//	  This prevents orphaned checkpoint data from accumulating.
//
// Multi-tenant:
//   - All graph instances across all tenants share one KV bucket.
//   - Keys are differentiated by prefix: "{tenant_id}:{graph_instance_id}"
//   - PurgeTenant() deletes all keys matching a tenant prefix.
//
// Usage:
//
//	nc, _ := nats.Connect("nats://localhost:4222")
//	js, _ := jetstream.New(nc)
//	saver, _ := checkpoint.NewNATSSaver(js, &checkpoint.NATSConfig{
//	    Bucket:   "checkpoints",
//	    History:  3,
//	    Replicas: 1,
//	})
//	graph, _ := sg.Compile(graph.WithCheckpointer(saver))
type NATSSaver struct {
	js           jetstream.JetStream
	kv           jetstream.KeyValue
	bucket       string
	history      int
	replicas     int
	maxGraphIdle time.Duration

	mu      sync.Mutex
	stopped bool
	closeCh chan struct{}
	wg      sync.WaitGroup
}

// NATSConfig configures the NATS checkpoint saver.
type NATSConfig struct {
	// Bucket is the NATS KV bucket name. Default: "checkpoints".
	Bucket string

	// History is max versions per key. Each graph instance keeps only
	// this many recent checkpoints. Older versions are auto-evicted by NATS.
	// Default: 3.
	History int

	// Replicas is the number of replicas for the KV bucket.
	// 1 = R1 (fast, single node). 3 = R3 (production cluster). Default: 1.
	Replicas int

	// MaxGraphIdle controls when a graph instance is considered completed.
	// If a key's latest checkpoint is older than this, the background GC
	// will purge all checkpoints for that graph instance.
	// Active graphs checkpoint every few seconds, so they never trigger this.
	// Default: 30 minutes. 0 disables background GC.
	MaxGraphIdle time.Duration

	// GCInterval controls how often the background GC runs. Default: 10 minutes.
	GCInterval time.Duration
}

func (c *NATSConfig) defaults() {
	if c.Bucket == "" {
		c.Bucket = "checkpoints"
	}
	if c.History <= 0 {
		c.History = 3
	}
	if c.Replicas <= 0 {
		c.Replicas = 1
	}
	if c.MaxGraphIdle <= 0 {
		c.MaxGraphIdle = 30 * time.Minute
	}
	if c.GCInterval <= 0 {
		c.GCInterval = 10 * time.Minute
	}
}

// NewNATSSaver creates a NATS-backed checkpoint saver.
// The JetStream must already be created from an active NATS connection.
// Call Close() to stop background GC and release resources.
func NewNATSSaver(js jetstream.JetStream, cfg *NATSConfig) (*NATSSaver, error) {
	if cfg == nil {
		cfg = &NATSConfig{}
	}
	cfg.defaults()

	// Create or retrieve the KV bucket
	kv, err := js.CreateKeyValue(context.Background(), jetstream.KeyValueConfig{
		Bucket:      cfg.Bucket,
		Description: "Harness-Go checkpoint storage",
		History:     uint8(cfg.History),
		Replicas:    cfg.Replicas,
		Storage:     jetstream.FileStorage,
	})
	if err != nil {
		return nil, fmt.Errorf("create NATS KV bucket %q: %w", cfg.Bucket, err)
	}

	s := &NATSSaver{
		js:           js,
		kv:           kv,
		bucket:       cfg.Bucket,
		history:      cfg.History,
		replicas:     cfg.Replicas,
		maxGraphIdle: cfg.MaxGraphIdle,
		closeCh:      make(chan struct{}),
	}

	// Start background GC if enabled
	if cfg.MaxGraphIdle > 0 && cfg.GCInterval > 0 {
		s.wg.Add(1)
		go s.runGC(cfg.GCInterval)
	}

	return s, nil
}

// ---- Key encoding ----

// encodeKey builds the KV key for a checkpoint.
// Key format: "{tenant_id}:{thread_id}"
//
// NATS KV key restrictions: alphanumeric, dashes, underscores, equal signs, dots.
// Colon is acceptable. No spaces, no slashes.
func encodeKey(threadID string) string {
	return threadID
}

// ---- BaseCheckpointer implementation ----

// Get retrieves the latest checkpoint for a thread.
//
// Config keys:
//   - constants.ConfigKeyThreadID (string, required): thread ID.
func (s *NATSSaver) Get(ctx context.Context, config map[string]interface{}) (map[string]interface{}, error) {
	threadID, err := getStringConfig(config, constants.ConfigKeyThreadID)
	if err != nil {
		return nil, err
	}

	key := encodeKey(threadID)

	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("nats kv get %q: %w", key, err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(entry.Value(), &result); err != nil {
		return nil, fmt.Errorf("unmarshal checkpoint for %q: %w", key, err)
	}

	return result, nil
}

// Put saves a checkpoint for a thread.
//
// Config keys:
//   - constants.ConfigKeyThreadID (string, required): thread ID.
func (s *NATSSaver) Put(ctx context.Context, config map[string]interface{}, checkpoint map[string]interface{}) error {
	threadID, err := getStringConfig(config, constants.ConfigKeyThreadID)
	if err != nil {
		return err
	}

	key := encodeKey(threadID)

	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("marshal checkpoint for %q: %w", key, err)
	}

	_, err = s.kv.Put(ctx, key, data)
	if err != nil {
		return fmt.Errorf("nats kv put %q: %w", key, err)
	}

	return nil
}

// List returns a list of checkpoints for a thread.
//
// Config keys:
//   - constants.ConfigKeyThreadID (string, required): thread ID.
func (s *NATSSaver) List(ctx context.Context, config map[string]interface{}, limit int) ([]map[string]interface{}, error) {
	threadID, err := getStringConfig(config, constants.ConfigKeyThreadID)
	if err != nil {
		return nil, err
	}

	key := encodeKey(threadID)

	entries, err := s.kv.History(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("nats kv history %q: %w", key, err)
	}

	var results []map[string]interface{}
	for i := len(entries) - 1; i >= 0 && len(results) < limit; i-- {
		entry := entries[i]

		// Skip delete markers
		if entry.Operation() == jetstream.KeyValuePurge || entry.Operation() == jetstream.KeyValueDelete {
			continue
		}

		var cp map[string]interface{}
		if err := json.Unmarshal(entry.Value(), &cp); err != nil {
			continue
		}

		result := map[string]interface{}{
			"checkpoint_id": cp["id"],
			"thread_id":     threadID,
			"metadata":      cp["metadata"],
			"created_at":    entry.Created().Format(time.RFC3339Nano),
			"parent_id":     cp["parent_id"],
			"revision":      entry.Revision(),
		}
		results = append(results, result)
	}

	return results, nil
}

// runGC periodically scans for completed graph instances and purges them.
func (s *NATSSaver) runGC(interval time.Duration) {
	defer s.wg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.closeCh:
			return
		case <-ticker.C:
			s.collectGarbage()
		}
	}
}

// collectGarbage scans all keys and purges graph instances that are idle.
// An idle graph instance = a completed/abandoned graph = eligible for cleanup.
func (s *NATSSaver) collectGarbage() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	keys, err := s.kv.Keys(ctx)
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-s.maxGraphIdle)
	var purged int

	for _, key := range keys {
		select {
		case <-ctx.Done():
			return
		default:
		}

		entry, err := s.kv.Get(ctx, key)
		if err != nil {
			continue
		}

		if entry.Created().Before(cutoff) {
			if err = s.kv.Delete(ctx, key); err == nil {
				purged++
			}
		}
	}

	if purged > 0 {
		common.Info("NATSSaver GC purged completed graph instances", zap.Int("purged", purged))
	}
}

// PurgeTenant deletes all checkpoint data for a specific tenant.
func (s *NATSSaver) PurgeTenant(ctx context.Context, tenantID string) (int, error) {
	prefix := tenantID + ":"

	keys, err := s.kv.Keys(ctx)
	if err != nil {
		return 0, fmt.Errorf("list keys for tenant %q: %w", tenantID, err)
	}

	var purged int
	for _, key := range keys {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			if err := s.kv.Delete(ctx, key); err == nil {
				purged++
			}
		}
	}

	return purged, nil
}

// PurgeThread deletes checkpoint data for a specific thread.
func (s *NATSSaver) PurgeThread(ctx context.Context, threadID string) error {
	return s.kv.Delete(ctx, encodeKey(threadID))
}

// Close stops background GC and releases resources.
func (s *NATSSaver) Close() error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return nil
	}
	s.stopped = true
	close(s.closeCh)
	s.mu.Unlock()

	s.wg.Wait()
	return nil
}

// ---- Helpers ----

func getStringConfig(config map[string]interface{}, key string) (string, error) {
	if config == nil {
		return "", fmt.Errorf("config is nil, missing required key %q", key)
	}
	v, ok := config[key]
	if !ok {
		return "", fmt.Errorf("config missing required key %q", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("config key %q is not a string (got %T)", key, v)
	}
	if s == "" {
		return "", fmt.Errorf("config key %q is empty", key)
	}
	return s, nil
}
