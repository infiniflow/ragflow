package cache

// SwapGlobalClientForTest swaps the global Redis client and returns a restore
// function. This is intended for deterministic tests that need to control cache
// availability.
func SwapGlobalClientForTest(client *RedisClient) func() {
	prev := globalClient
	globalClient = client
	return func() {
		globalClient = prev
	}
}

