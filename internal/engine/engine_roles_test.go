package engine

import "testing"

// TestDocEngineIsCompositionOfRoles pins the ISP decomposition as a contract:
// DocEngine must be satisfied by every role interface (each role's method set is
// a subset of DocEngine), and any value carrying all six roles must satisfy
// DocEngine (the roles together cover DocEngine). Together these assert
// DocEngine == union(roles), so narrowing a consumer to a single role can never
// hide a method that consumer still needs, and adding a method to DocEngine
// without placing it in a role breaks this test.
func TestDocEngineIsCompositionOfRoles(t *testing.T) {
	var de DocEngine
	var _ ChunkStore = de
	var _ Searcher = de
	var _ MetadataStore = de
	var _ DocIndexer = de
	var _ SQLRunner = de
	var _ EngineLifecycle = de

	// The reverse direction: a value implementing all six roles satisfies
	// DocEngine, i.e. the roles cover every DocEngine method.
	var composed interface {
		ChunkStore
		Searcher
		MetadataStore
		DocIndexer
		SQLRunner
		EngineLifecycle
	}
	var _ DocEngine = composed
}
