package admin

import (
	"testing"
	"time"
)

func TestGetHeartbeatStatus(t *testing.T) {
	now := time.Date(2026, time.November, 4, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name     string
		interval time.Duration
		age      time.Duration
		want     string
	}{
		{name: "default before threshold", interval: 3 * time.Second, age: 15*time.Second - time.Nanosecond, want: "alive"},
		{name: "default at threshold", interval: 3 * time.Second, age: 15 * time.Second, want: "timeout"},
		{name: "default after threshold", interval: 3 * time.Second, age: 15*time.Second + time.Nanosecond, want: "timeout"},
		{name: "long interval before threshold", interval: 40 * time.Second, age: 200*time.Second - time.Nanosecond, want: "alive"},
		{name: "long interval at threshold", interval: 40 * time.Second, age: 200 * time.Second, want: "timeout"},
		{name: "long interval after threshold", interval: 40 * time.Second, age: 200*time.Second + time.Nanosecond, want: "timeout"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := getHeartbeatStatus(now.Add(-tt.age), now, tt.interval); got != tt.want {
				t.Fatalf("heartbeat age %s with interval %s = %q, want %q", tt.age, tt.interval, got, tt.want)
			}
		})
	}
}
