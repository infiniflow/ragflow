package service

import "fmt"

// IngestionLogSettings controls event message bounds and terminal-run
// retention. The defaults match the rollout plan and are also used by tests
// and callers that construct an IngestionTaskService directly.
type IngestionLogSettings struct {
	MaxRowsPerRun      int
	MaxRowsPerDocument int
	MaxMessageChars    int
	MaxMessageBytes    int
}

func DefaultIngestionLogSettings() IngestionLogSettings {
	return IngestionLogSettings{
		MaxRowsPerRun:      5_000,
		MaxRowsPerDocument: 20_000,
		MaxMessageChars:    4_000,
		MaxMessageBytes:    16_384,
	}
}

func (s IngestionLogSettings) validate() error {
	if s.MaxRowsPerRun < 2 {
		return fmt.Errorf("ingestion log max rows per run must be at least 2")
	}
	if s.MaxRowsPerDocument < s.MaxRowsPerRun {
		return fmt.Errorf("ingestion log max rows per document must be >= max rows per run")
	}
	if s.MaxMessageChars <= 0 {
		return fmt.Errorf("ingestion log max message chars must be positive")
	}
	if s.MaxMessageBytes <= 0 {
		return fmt.Errorf("ingestion log max message bytes must be positive")
	}
	return nil
}

// SetIngestionLogSettings validates and replaces the service's event and
// retention limits. It is intended to be called during startup before the
// service is used by workers.
func (s *IngestionTaskService) SetIngestionLogSettings(settings IngestionLogSettings) error {
	if err := settings.validate(); err != nil {
		return err
	}
	s.logSettings = settings
	return nil
}
