# query-cancellation: Implement Query Cancellation Properly

## Priority: MEDIUM

## Explanation

TanStack Query provides an `AbortSignal` to cancel in-flight requests when queries become stale or components unmount. Pass this signal to your fetch calls to prevent memory leaks and wasted bandwidth.

## Bad Example

```tsx
// Not using abort signal - requests complete even when unnecessary
const { data } = useQuery({
  queryKey: ['search', searchTerm],
  queryFn: async () => {
    // User types fast: "a", "ab", "abc"
    // Three requests fire, all complete, wasting bandwidth
    const response = await fetch(`/api/search?q=${searchTerm}`)
    return response.json()
  },
})

// Component unmounts but request keeps running
function UserProfile({ userId }: { userId: string }) {
  const { data } = useQuery({
    queryKey: ['user', userId],
    queryFn: async () => {
      const response = await fetch(`/api/users/${userId}`)
      return response.json()  // Completes even if user navigated away
    },
  })
}
```

## Good Example: Using AbortSignal with Fetch

```tsx
const { data } = useQuery({
  queryKey: ['search', searchTerm],
  queryFn: async ({ signal }) => {
    const response = await fetch(`/api/search?q=${searchTerm}`, {
      signal,  // Pass abort signal to fetch
    })
    return response.json()
  },
})

// Now when user types "a", "ab", "abc" quickly:
// - "a" request is cancelled when "ab" starts
// - "ab" request is cancelled when "abc" starts
// - Only "abc" completes
```

## Good Example: With Axios

```tsx
import axios from 'axios'

const { data } = useQuery({
  queryKey: ['users', userId],
  queryFn: async ({ signal }) => {
    const response = await axios.get(`/api/users/${userId}`, {
      signal,  // Axios supports AbortSignal
    })
    return response.data
  },
})
```

## Good Example: Manual Cancellation

```tsx
function SearchResults() {
  const queryClient = useQueryClient()
  const [searchTerm, setSearchTerm] = useState('')

  const { data } = useQuery({
    queryKey: ['search', searchTerm],
    queryFn: async ({ signal }) => {
      const response = await fetch(`/api/search?q=${searchTerm}`, { signal })
      return response.json()
    },
    enabled: searchTerm.length > 0,
  })

  // Cancel all search queries manually
  const handleClear = () => {
    queryClient.cancelQueries({ queryKey: ['search'] })
    setSearchTerm('')
  }

  return (
    <div>
      <input
        value={searchTerm}
        onChange={(e) => setSearchTerm(e.target.value)}
      />
      <button onClick={handleClear}>Clear</button>
      <Results data={data} />
    </div>
  )
}
```

## Good Example: In Mutations (Before Optimistic Update)

```tsx
const updateTodo = useMutation({
  mutationFn: (todo: Todo) => api.updateTodo(todo),
  onMutate: async (newTodo) => {
    // Cancel outgoing queries to prevent overwriting optimistic update
    await queryClient.cancelQueries({ queryKey: ['todos'] })
    await queryClient.cancelQueries({ queryKey: ['todos', newTodo.id] })

    // Proceed with optimistic update...
    const previousTodos = queryClient.getQueryData(['todos'])
    queryClient.setQueryData(['todos'], (old) => /* ... */)

    return { previousTodos }
  },
})
```

## Good Example: Custom Cancellable Promise

```tsx
// For non-fetch APIs that need custom cancellation
const { data } = useQuery({
  queryKey: ['expensive-computation', params],
  queryFn: ({ signal }) => {
    return new Promise((resolve, reject) => {
      // Check if already cancelled
      if (signal.aborted) {
        reject(new DOMException('Aborted', 'AbortError'))
        return
      }

      const worker = new Worker('computation.js')
      worker.postMessage(params)

      worker.onmessage = (e) => resolve(e.data)
      worker.onerror = (e) => reject(e)

      // Listen for cancellation
      signal.addEventListener('abort', () => {
        worker.terminate()
        reject(new DOMException('Aborted', 'AbortError'))
      })
    })
  },
})
```

## When Queries Are Cancelled

| Scenario | Cancelled? |
|----------|------------|
| Query key changes | Yes |
| Component unmounts | Yes |
| `queryClient.cancelQueries()` called | Yes |
| Refetch triggered | Previous request cancelled |
| `enabled` becomes false | Yes |

## Context

- Always pass `signal` to fetch/axios for automatic cancellation
- Cancelled queries don't trigger `onError` - they're silently dropped
- Use `queryClient.cancelQueries()` before optimistic updates
- AbortError is thrown when cancelled - handle if needed
- Cancellation prevents wasted bandwidth and race conditions
- Essential for search-as-you-type and fast navigation patterns
