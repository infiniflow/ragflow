//go:build !ee

package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// TestIngestionLogConfigStubIsInertNoSection: in the OSS (non-ee) build the
// ingestion-log parse is a no-op stub. With no [ingestion_log] section it must
// leave Type empty and return nil - it must never default to "clickhouse" or
// otherwise activate the EE ClickHouse sink.
func TestIngestionLogConfigStubIsInertNoSection(t *testing.T) {
	c := &Config{}
	if err := c.ParseIngestLogConfig(viper.New()); err != nil {
		t.Fatalf("non-ee ParseIngestLogConfig must be a no-op: %v", err)
	}
	if c.ingestLog.Type != "" {
		t.Fatalf("non-ee stub must not set Type, got %q", c.ingestLog.Type)
	}
}

// TestIngestionLogConfigStubIgnoresClickHouse: even an explicit
// [ingestion_log].type: clickhouse must be ignored by the OSS stub, so the EE
// feature can never be switched on from OSS-only code.
func TestIngestionLogConfigStubIgnoresClickHouse(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(strings.NewReader("ingestion_log:\n  type: clickhouse\n")); err != nil {
		t.Fatalf("read config: %v", err)
	}
	c := &Config{}
	if err := c.ParseIngestLogConfig(v); err != nil {
		t.Fatalf("non-ee ParseIngestLogConfig must be a no-op: %v", err)
	}
	if c.ingestLog.Type != "" {
		t.Fatalf("non-ee stub must ignore [ingestion_log].type, got %q", c.ingestLog.Type)
	}
}
