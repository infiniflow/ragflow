# cache-invalidation: Use Targeted Invalidation Over Broad Patterns

## Priority: CRITICAL

## Explanation

Query invalidation marks cached data as stale, triggering background refetches. Use targeted invalidation to refresh only affected data. Overly broad invalidation causes unnecessary network requests; too narrow invalidation leaves stale data.

## Bad Example

```tsx
// Invalidating everything after a single todo update
const mutation = useMutation({
  mutationFn: updateTodo,
  onSuccess: () => {
    queryClient.invalidateQueries()  // Invalidates ENTIRE cache
  },
})

// Invalidating too broadly
const mutation = useMutation({
  mutationFn: updateTodoStatus,
  onSuccess: () => {
    // Invalidates all todos including unrelated lists
    queryClient.invalidateQueries({ queryKey: ['todos'] })
  },
})

// Missing invalidation of related queries
const mutation = useMutation({
  mutationFn: addComment,
  onSuccess: () => {
    // Only invalidates comment list, misses comment count
    queryClient.invalidateQueries({ queryKey: ['comments', postId] })
  },
})
```

## Good Example

```tsx
// Targeted invalidation with exact matching
const mutation = useMutation({
  mutationFn: updateTodo,
  onSuccess: (data, variables) => {
    // Invalidate specific todo and related queries
    queryClient.invalidateQueries({ queryKey: ['todos', variables.id] })
    // Also invalidate lists that might contain this todo
    queryClient.invalidateQueries({ queryKey: ['todos', 'list'] })
  },
})

// Use exact: true when you only want one specific query
const mutation = useMutation({
  mutationFn: updateUserProfile,
  onSuccess: () => {
    queryClient.invalidateQueries({
      queryKey: ['user', 'profile'],
      exact: true,  // Only this exact key, not ['user', 'profile', 'settings']
    })
  },
})

// Invalidate multiple related queries
const mutation = useMutation({
  mutationFn: addComment,
  onSuccess: (data, { postId }) => {
    // Invalidate all comment-related queries for this post
    queryClient.invalidateQueries({ queryKey: ['posts', postId, 'comments'] })
    queryClient.invalidateQueries({ queryKey: ['posts', postId, 'comment-count'] })
    // Optionally invalidate the post itself if it shows comment count
    queryClient.invalidateQueries({ queryKey: ['posts', postId] })
  },
})

// Predicate-based invalidation for complex scenarios
queryClient.invalidateQueries({
  predicate: (query) =>
    query.queryKey[0] === 'todos' &&
    query.state.data?.userId === currentUserId,
})
```

## Invalidation Patterns

```tsx
// Prefix matching (default) - invalidates all matching prefixes
queryClient.invalidateQueries({ queryKey: ['todos'] })
// Matches: ['todos'], ['todos', 1], ['todos', { status: 'done' }]

// Exact matching - only the exact key
queryClient.invalidateQueries({ queryKey: ['todos'], exact: true })
// Matches: ['todos'] only

// Predicate matching - custom logic
queryClient.invalidateQueries({
  predicate: (query) => query.queryKey.includes('user-generated'),
})

// Refetch type control
queryClient.invalidateQueries({
  queryKey: ['todos'],
  refetchType: 'active',  // Only refetch active queries (default)
  // refetchType: 'inactive' - Only inactive
  // refetchType: 'all' - Both
  // refetchType: 'none' - Mark stale but don't refetch
})
```

## Context

- Invalidation only marks queries as stale; refetch happens when query is used
- `refetchType: 'active'` (default) only refetches queries with active observers
- Use hierarchical query keys to enable precise invalidation
- Consider `setQueryData` for optimistic updates instead of invalidation
- Always test invalidation patterns to ensure all affected queries are refreshed
