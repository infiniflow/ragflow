package workflowx

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// R3 stubs — the r3 interrupt/resume tests were written for the eino
// framework which is not available in this branch. These stubs make
// the file compile without deleting the test logic.

type inMemoryStore struct{ data map[string][]byte }

func newInMemoryStore() *inMemoryStore { return &inMemoryStore{data: map[string][]byte{}} }

func (s *inMemoryStore) Get(_ context.Context, id string) ([]byte, bool, error) {
	v, ok := s.data[id]
	return v, ok, nil
}
func (s *inMemoryStore) Set(_ context.Context, id string, payload []byte) error {
	cp := make([]byte, len(payload))
	copy(cp, payload)
	s.data[id] = cp
	return nil
}
func (s *inMemoryStore) Delete(_ context.Context, id string) error {
	delete(s.data, id)
	return nil
}

// fakeEinoNode provides the methods the test file's compiled eino
// graph nodes expose (AddInput / End). The tests will skip at runtime.
type fakeEinoNode struct{ name string }

func (f *fakeEinoNode) AddInput(...string) {}
func (f *fakeEinoNode) End() *fakeEinoNode { return f }

func AddParallelNode(context.Context, any, string, any) (*fakeEinoNode, error) {
	return &fakeEinoNode{name: "parallel"}, nil
}

// NewInMemoryStore is the exported alias for tests outside the package.
func NewInMemoryStore() *inMemoryStore { return newInMemoryStore() }

// GetTime returns current UTC time for test assertions.
func GetTime() time.Time { return time.Now().UTC() }

// NewUUID returns a new UUID string.
func NewUUID() string { return uuid.New().String() }
