# qk-array-structure: Always Use Arrays for Query Keys

## Priority: CRITICAL

## Explanation

Query keys must always be arrays at the top level. This enables proper caching, invalidation matching, and query deduplication. Using non-array keys will cause unexpected behavior and cache misses.

## Bad Example

```tsx
// Never use strings or non-array types as query keys
const { data } = useQuery({
  queryKey: 'todos',  // Wrong: string instead of array
  queryFn: fetchTodos,
})

const { data: user } = useQuery({
  queryKey: { id: 1, type: 'user' },  // Wrong: object instead of array
  queryFn: fetchUser,
})
```

## Good Example

```tsx
// Always use arrays for query keys
const { data } = useQuery({
  queryKey: ['todos'],
  queryFn: fetchTodos,
})

const { data: user } = useQuery({
  queryKey: ['user', 1],
  queryFn: () => fetchUser(1),
})

// Complex keys with objects inside arrays are fine
const { data: filteredTodos } = useQuery({
  queryKey: ['todos', { status: 'done', page: 1 }],
  queryFn: () => fetchTodos({ status: 'done', page: 1 }),
})
```

## Context

- Always applicable when defining query keys
- Arrays enable prefix-based invalidation (e.g., `invalidateQueries({ queryKey: ['todos'] })` matches all todo queries)
- Object property order inside arrays doesn't matter for matching
- Array element order does matter: `['todos', 1]` !== `['1', 'todos']`
