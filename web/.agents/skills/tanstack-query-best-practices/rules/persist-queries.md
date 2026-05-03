# persist-queries: Configure Query Persistence for Offline Support

## Priority: LOW

## Explanation

TanStack Query can persist the cache to storage (localStorage, IndexedDB, AsyncStorage) and restore it on app load. This enables offline support and faster startup by eliminating initial loading states.

## Bad Example

```tsx
// No persistence - always starts fresh
const queryClient = new QueryClient()

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <MyApp />
    </QueryClientProvider>
  )
}

// User refreshes page:
// 1. Empty cache
// 2. Loading spinners everywhere
// 3. Refetch all data
// Poor offline experience
```

## Good Example: Basic Persistence with localStorage

```tsx
import { QueryClient } from '@tanstack/react-query'
import { createSyncStoragePersister } from '@tanstack/query-sync-storage-persister'
import { PersistQueryClientProvider } from '@tanstack/react-query-persist-client'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      gcTime: 1000 * 60 * 60 * 24,  // 24 hours - keep cache longer for persistence
      staleTime: 1000 * 60 * 5,     // 5 minutes
    },
  },
})

const persister = createSyncStoragePersister({
  storage: window.localStorage,
  key: 'REACT_QUERY_CACHE',
})

function App() {
  return (
    <PersistQueryClientProvider
      client={queryClient}
      persistOptions={{
        persister,
        maxAge: 1000 * 60 * 60 * 24,  // 24 hours max
      }}
    >
      <MyApp />
    </PersistQueryClientProvider>
  )
}
```

## Good Example: Async Persistence with IndexedDB

```tsx
import { createAsyncStoragePersister } from '@tanstack/query-async-storage-persister'
import { get, set, del } from 'idb-keyval'

const persister = createAsyncStoragePersister({
  storage: {
    getItem: async (key) => await get(key),
    setItem: async (key, value) => await set(key, value),
    removeItem: async (key) => await del(key),
  },
  key: 'REACT_QUERY_CACHE',
})

function App() {
  return (
    <PersistQueryClientProvider
      client={queryClient}
      persistOptions={{
        persister,
        maxAge: 1000 * 60 * 60 * 24 * 7,  // 7 days
        buster: APP_VERSION,  // Bust cache on app updates
      }}
    >
      <MyApp />
    </PersistQueryClientProvider>
  )
}
```

## Good Example: Selective Persistence

```tsx
import { persistQueryClient } from '@tanstack/react-query-persist-client'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      gcTime: 1000 * 60 * 60 * 24,
    },
  },
})

// Only persist certain queries
persistQueryClient({
  queryClient,
  persister,
  dehydrateOptions: {
    shouldDehydrateQuery: (query) => {
      // Don't persist user-specific sensitive data
      if (query.queryKey[0] === 'user-session') return false
      // Don't persist real-time data
      if (query.queryKey[0] === 'notifications') return false
      // Don't persist failed queries
      if (query.state.status !== 'success') return false
      // Persist everything else
      return true
    },
  },
})
```

## Good Example: React Native with AsyncStorage

```tsx
import AsyncStorage from '@react-native-async-storage/async-storage'
import { createAsyncStoragePersister } from '@tanstack/query-async-storage-persister'

const persister = createAsyncStoragePersister({
  storage: AsyncStorage,
  key: 'app-query-cache',
})

// Usage is the same as web
```

## Good Example: Handling Restoration Loading

```tsx
import { PersistQueryClientProvider } from '@tanstack/react-query-persist-client'

function App() {
  return (
    <PersistQueryClientProvider
      client={queryClient}
      persistOptions={{ persister }}
      onSuccess={() => {
        // Cache restored successfully
        console.log('Cache restored')
      }}
    >
      {/* Show loading while restoring */}
      <PersistQueryClientProvider.Consumer>
        {({ isRestoring }) =>
          isRestoring ? <SplashScreen /> : <MainApp />
        }
      </PersistQueryClientProvider.Consumer>
    </PersistQueryClientProvider>
  )
}

// Or use the hook
function MainApp() {
  const { isRestoring } = usePersistQueryClientRestore()

  if (isRestoring) return <SplashScreen />
  return <App />
}
```

## Persistence Configuration

| Option | Purpose |
|--------|---------|
| `maxAge` | Maximum cache age before considered invalid |
| `buster` | String to invalidate cache (use app version) |
| `dehydrateOptions.shouldDehydrateQuery` | Filter which queries to persist |
| `hydrateOptions.shouldHydrate` | Filter which queries to restore |

## Context

- Requires `@tanstack/react-query-persist-client` package
- Set `gcTime` higher than default (5 min) for persistence to be useful
- Use `buster` option to invalidate cache on app updates
- Don't persist sensitive data or real-time data
- IndexedDB is better than localStorage for large caches
- Restored data is still subject to staleTime checks
- Works well with `networkMode: 'offlineFirst'`
