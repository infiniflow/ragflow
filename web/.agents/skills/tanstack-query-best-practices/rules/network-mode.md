# network-mode: Configure Network Mode for Offline Support

## Priority: LOW

## Explanation

TanStack Query's `networkMode` controls how queries and mutations behave when there's no network connection. Configure it based on your app's offline requirements: always fetch, pause when offline, or work entirely offline.

## Bad Example

```tsx
// Not considering offline behavior
const { data } = useQuery({
  queryKey: ['todos'],
  queryFn: fetchTodos,
  // Default networkMode: 'online'
  // Query pauses with no feedback when offline
})

// User goes offline, sees stale data with no indication
// Mutations silently queue with no UI feedback
```

## Good Example: Default Online Mode with Offline UI

```tsx
// Show clear offline state to users
function TodoList() {
  const { data, fetchStatus, status } = useQuery({
    queryKey: ['todos'],
    queryFn: fetchTodos,
    networkMode: 'online',  // Default - pauses when offline
  })

  // fetchStatus: 'fetching' | 'paused' | 'idle'
  // 'paused' means waiting for network

  return (
    <div>
      {fetchStatus === 'paused' && (
        <Banner>You're offline. Showing cached data.</Banner>
      )}
      <TodoItems todos={data} />
    </div>
  )
}
```

## Good Example: Always Mode for Offline-First

```tsx
// App works offline with local data
const { data, error } = useQuery({
  queryKey: ['todos'],
  queryFn: async () => {
    // Try network first
    try {
      const todos = await fetchTodosFromServer()
      await saveToLocalDB(todos)  // Sync to local
      return todos
    } catch (e) {
      // Fall back to local data
      return getFromLocalDB()
    }
  },
  networkMode: 'always',  // Always runs queryFn, even offline
})

// Or set globally
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      networkMode: 'always',
    },
    mutations: {
      networkMode: 'always',
    },
  },
})
```

## Good Example: Offline-First Mode

```tsx
// Only fetch when online, but don't fail when offline
const { data } = useQuery({
  queryKey: ['user-preferences'],
  queryFn: fetchPreferences,
  networkMode: 'offlineFirst',
  // Runs queryFn once, then waits for network if it fails
  // Good for: data that's useful to attempt offline
})
```

## Good Example: Mutation Offline Queue

```tsx
function TodoApp() {
  const queryClient = useQueryClient()

  const addTodo = useMutation({
    mutationFn: createTodo,
    networkMode: 'online',  // Pauses when offline
    onMutate: async (newTodo) => {
      // Optimistic update works offline
      await queryClient.cancelQueries({ queryKey: ['todos'] })
      const previous = queryClient.getQueryData(['todos'])
      queryClient.setQueryData(['todos'], (old: Todo[]) => [...old, newTodo])
      return { previous }
    },
    onError: (err, newTodo, context) => {
      queryClient.setQueryData(['todos'], context?.previous)
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ['todos'] })
    },
  })

  // Track paused mutations
  const pendingMutations = useMutationState({
    filters: { status: 'pending' },
  })

  const pausedMutations = pendingMutations.filter(
    m => m.state.isPaused
  )

  return (
    <div>
      {pausedMutations.length > 0 && (
        <Banner>
          {pausedMutations.length} changes waiting to sync
        </Banner>
      )}
      <TodoList />
    </div>
  )
}
```

## Network Mode Comparison

| Mode | Behavior | Use Case |
|------|----------|----------|
| `'online'` (default) | Pauses when offline, resumes when online | Most apps, show offline state |
| `'always'` | Always runs queryFn regardless of network | Offline-first apps, local-only data |
| `'offlineFirst'` | Tries once, then waits for network if fails | Best-effort offline |

## Good Example: Online Status Detection

```tsx
import { onlineManager } from '@tanstack/react-query'

// React to online/offline changes
function NetworkStatus() {
  const isOnline = useSyncExternalStore(
    onlineManager.subscribe,
    () => onlineManager.isOnline(),
  )

  return (
    <div className={isOnline ? 'online' : 'offline'}>
      {isOnline ? 'Connected' : 'Offline'}
    </div>
  )
}

// Manually override online detection (for testing)
onlineManager.setOnline(false)
```

## Context

- Default `'online'` mode is best for most apps
- `fetchStatus: 'paused'` indicates waiting for network
- Mutations queue automatically and retry when back online
- Use `onlineManager` to detect and control online state
- Combine with optimistic updates for seamless offline UX
- Consider service workers for true offline support
