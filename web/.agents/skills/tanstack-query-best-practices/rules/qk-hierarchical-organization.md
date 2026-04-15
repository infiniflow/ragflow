# qk-hierarchical-organization: Organize Keys Hierarchically

## Priority: CRITICAL

## Explanation

Structure query keys from general to specific: entity type first, then ID, then modifiers/filters. This enables efficient invalidation at any level of specificity and creates predictable cache organization.

## Bad Example

```tsx
// Flat, inconsistent key structures
const { data: todos } = useQuery({
  queryKey: ['all-todos-list'],
  queryFn: fetchTodos,
})

const { data: todo } = useQuery({
  queryKey: ['single-todo-5'],
  queryFn: () => fetchTodo(5),
})

const { data: comments } = useQuery({
  queryKey: ['todo-5-comments'],
  queryFn: () => fetchTodoComments(5),
})

// Can't easily invalidate all todo-related queries
```

## Good Example

```tsx
// Hierarchical: entity → id → sub-resource → filters
const { data: todos } = useQuery({
  queryKey: ['todos'],
  queryFn: fetchTodos,
})

const { data: todo } = useQuery({
  queryKey: ['todos', 5],
  queryFn: () => fetchTodo(5),
})

const { data: comments } = useQuery({
  queryKey: ['todos', 5, 'comments'],
  queryFn: () => fetchTodoComments(5),
})

const { data: filteredTodos } = useQuery({
  queryKey: ['todos', { status: 'done', page: 1 }],
  queryFn: () => fetchTodos({ status: 'done', page: 1 }),
})

// Now we can invalidate at any level:
queryClient.invalidateQueries({ queryKey: ['todos'] })        // All todos
queryClient.invalidateQueries({ queryKey: ['todos', 5] })     // Todo 5 and its sub-resources
queryClient.invalidateQueries({ queryKey: ['todos', 5, 'comments'] }) // Just comments
```

## Recommended Hierarchy Pattern

```
['entity']                              // List
['entity', id]                          // Single item
['entity', id, 'sub-resource']          // Related data
['entity', { filters }]                 // Filtered list
['entity', id, 'sub-resource', { filters }] // Filtered sub-resource
```

## Context

- Essential for applications with related data
- Enables efficient cache management
- Works with prefix-based invalidation
- Consider using query key factories (see `qk-factory-pattern`) for consistency
