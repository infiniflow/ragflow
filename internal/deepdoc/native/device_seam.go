//go:build cgo

package native

import (
	"fmt"
	"strconv"
	"strings"

	ort "github.com/infiniflow/onnxruntime_go"
)

// DeviceConfig is the runtime device policy for DeepDoc inference. The shared
// build (no `ee` tag) always uses the zero value (CPU). The EE build fills it
// from DEEPDOC_DEVICE / --deepdoc-device via device_seam_ee.go.
type DeviceConfig struct {
	UseGPU        bool
	DeviceID      int
	MemLimitMB    int
	EPLibraryPath string
}

// String renders the device in canonical cpu|gpu[:id] form (shared, no CUDA).
func (d DeviceConfig) String() string {
	if !d.UseGPU {
		return "cpu"
	}
	if d.DeviceID == 0 {
		return "gpu:0"
	}
	return "gpu:" + strconv.Itoa(d.DeviceID)
}

// deviceConfig holds the registered policy (atomic-safe immutable value).
var deviceConfigHolder atomicDeviceConfig

// getDevice returns the current device policy. Shared build returns CPU.
func getDevice() DeviceConfig { return deviceConfigHolder.load() }

// SetDevice registers the process-wide device policy. In the shared (non-EE)
// build GPU is ignored — the backend always runs on CPU.
func SetDevice(cfg DeviceConfig) {
	if !gpuAvailable {
		cfg = DeviceConfig{} // force CPU in OSS/non-EE build
	}
	if cfg.MemLimitMB < 0 {
		cfg.MemLimitMB = 0
	}
	if cfg.DeviceID < 0 {
		cfg.DeviceID = 0
	}
	deviceConfigHolder.store(cfg)
}

// gpuAvailable is set true only by the EE build (device_seam_ee.go init).
var gpuAvailable bool

// applyDeviceHook applies the device-specific execution provider to session
// options. No-op in the shared build; EE build installs the CUDA EP here.
var applyDeviceHook = func(opts *ort.SessionOptions) error { return nil }

// registerEPHook loads the device-specific EP plugin after InitORT. No-op in
// the shared build; EE build installs the CUDA EP .so loader here.
var registerEPHook = func() error { return nil }

// parseDeviceHook parses a device spec. The default (shared/OSS) build accepts
// only CPU and REJECTS any gpu request with a clear "not supported" error; the
// EE build overrides this hook in device_seam_ee.go to accept cpu|gpu[:id].
var parseDeviceHook = func(spec string) (DeviceConfig, error) {
	s := strings.ToLower(strings.TrimSpace(spec))
	if s == "" || s == "cpu" {
		return DeviceConfig{}, nil
	}
	return DeviceConfig{}, fmt.Errorf("deepdoc device %q is not supported in this build: GPU requires the enterprise edition (use cpu)", spec)
}

// ParseDevice parses a device spec. Shared/OSS build accepts only cpu (gpu ->
// error); the EE build accepts cpu|gpu[:id].
func ParseDevice(spec string) (DeviceConfig, error) { return parseDeviceHook(spec) }

// ApplyDeviceOptions is called by newSessionOptions to attach the selected EP.
func ApplyDeviceOptions(opts *ort.SessionOptions) error { return applyDeviceHook(opts) }

// RegisterExecutionProvider loads any runtime EP plugin. Called once at
// startup after InitORT; no-op on the shared/CPU build.
func RegisterExecutionProvider() error { return registerEPHook() }
