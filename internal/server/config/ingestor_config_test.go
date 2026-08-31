package config

import (
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestParseIngestorConfigDefaultsLeaseSettings(t *testing.T) {
	v := viper.New()
	c := &Config{}
	if err := c.ParseGeneralConfig(v); err != nil {
		t.Fatalf("ParseGeneralConfig: %v", err)
	}
	if err := c.ParseIngestorConfig(v); err != nil {
		t.Fatalf("ParseIngestorConfig: %v", err)
	}

	got := c.GetIngestorConfig()
	if got.ClaimTTL != 15*time.Second || got.MaxLeaseRecoveryAttempts != 3 {
		t.Fatalf("default lease config = ttl:%s max:%d, want ttl:15s max:3", got.ClaimTTL, got.MaxLeaseRecoveryAttempts)
	}
}

func TestParseIngestorConfigReadsLeaseSettings(t *testing.T) {
	v := viper.New()
	v.Set("general.heartbeat_interval", "2s")
	v.Set("ingestor.claim_ttl", "7s")
	v.Set("ingestor.max_lease_recovery_attempts", 5)
	c := &Config{}
	if err := c.ParseGeneralConfig(v); err != nil {
		t.Fatalf("ParseGeneralConfig: %v", err)
	}
	if err := c.ParseIngestorConfig(v); err != nil {
		t.Fatalf("ParseIngestorConfig: %v", err)
	}

	got := c.GetIngestorConfig()
	if got.ClaimTTL != 7*time.Second || got.MaxLeaseRecoveryAttempts != 5 {
		t.Fatalf("configured lease config = ttl:%s max:%d, want ttl:7s max:5", got.ClaimTTL, got.MaxLeaseRecoveryAttempts)
	}
}

func TestParseIngestorConfigRejectsUnsafeLeaseTTL(t *testing.T) {
	v := viper.New()
	v.Set("general.heartbeat_interval", "2s")
	v.Set("ingestor.claim_ttl", "6s")
	c := &Config{}
	if err := c.ParseGeneralConfig(v); err != nil {
		t.Fatalf("ParseGeneralConfig: %v", err)
	}
	if err := c.ParseIngestorConfig(v); err == nil {
		t.Fatal("claim TTL equal to three heartbeat intervals must be rejected")
	}
}
