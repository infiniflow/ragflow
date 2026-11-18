package wikisearch

import "sync"

var (
	svcMu   sync.RWMutex
	svcInst Service
)

// SetService installs the (production or test) wiki-search service singleton.
// Passing nil reverts to "not configured" (GetService returns nil), which is the
// pre-wiring state: the wiki_query tool then returns an empty result so the agent
// falls back to hybrid search.
func SetService(s Service) {
	svcMu.Lock()
	defer svcMu.Unlock()
	svcInst = s
}

// GetService returns the installed wiki-search service. It may be nil until
// SetService is called during server bootstrap.
func GetService() Service {
	svcMu.RLock()
	defer svcMu.RUnlock()
	return svcInst
}
