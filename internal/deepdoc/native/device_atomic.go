//go:build cgo

package native

import "sync/atomic"

// atomicDeviceConfig wraps atomic.Value with a typed DeviceConfig, so the
// shared seam file stays free of the atomic import detail.
type atomicDeviceConfig struct{ v atomic.Value }

func (a *atomicDeviceConfig) load() DeviceConfig {
	v, _ := a.v.Load().(DeviceConfig)
	return v
}
func (a *atomicDeviceConfig) store(c DeviceConfig) { a.v.Store(c) }

func init() { deviceConfigHolder.store(DeviceConfig{}) }
