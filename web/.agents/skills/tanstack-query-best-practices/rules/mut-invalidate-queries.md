# mut-invalidate-queries: Always Invalidate Related Queries After Mutations

## Priority: HIGH

## Explanation

After mutations, invalidate all queries whose data might be affected. This ensures the cache stays synchronized with the server. Forgetting to invalidate related queries leads to stale UI data.

## Bad Example

```tsx
// No invalidation - cache remains stale
const createTodo = useMutation({
  mutationFn: (newTodo) => api.createTodo(newTodo),
  // Missing onSuccess handler - todo list won't show new item
})

// Partial invalidation - misses related queries
const deleteTodo = useMutation({
  mutationFn: (todoId) => api.deleteTodo(todoId),
  onSuccess: () => {
    // Only invalidates list, not summary/counts
    queryClient.invalidateQueries({ queryKey: ['todos', 'list'] })
    // Missing: ['todos', 'count'], ['todos', 'completed-count'], etc.
  },
})
```

## Good Example

```tsx
// Comprehensive invalidation
const createTodo = useMutation({
  mutationFn: (newTodo) => api.createTodo(newTodo),
  onSuccess: () => {
    // Invalidate all todo-related queries
    queryClient.invalidateQueries({ queryKey: ['todos'] })
  },
})

// Targeted invalidation with all affected queries
const updateTodo = useMutation({
  mutationFn: ({ id, data }) => api.updateTodo(id, data),
  onSuccess: (data, { id }) => {
    // Specific todo
    queryClient.invalidateQueries({ queryKey: ['todos', id] })
    // Lists that might contain this todo
    queryClient.invalidateQueries({ queryKey: ['todos', 'list'] })
    // If todo status changed, invalidate filtered views
    queryClient.invalidateQueries({ queryKey: ['todos', 'completed'] })
    queryClient.invalidateQueries({ queryKey: ['todos', 'active'] })
  },
})

// Cross-entity invalidation
const assignTodoToUser = useMutation({
  mutationFn: ({ todoId, userId }) => api.assignTodo(todoId, userId),
  onSuccess: (data, { todoId, userId }) => {
    // Invalidate the todo
    queryClient.invalidateQueries({ queryKey: ['todos', todoId] })
    // Invalidate user's assigned todos
    queryClient.invalidateQueries({ queryKey: ['users', userId, 'todos'] })
    // Invalidate previous assignee's list if available
    if (data.previousAssignee) {
      queryClient.invalidateQueries({
        queryKey: ['users', data.previousAssignee, 'todos'],
      })
    }
  },
})
```

## Pattern: Mutation with Variables Access

```tsx
const mutation = useMutation({
  mutationFn: updatePost,
  onSuccess: (
    data,      // Server response
    variables, // What you passed to mutate()
    context    // What onMutate returned
  ) => {
    // Use variables to know which queries to invalidate
    queryClient.invalidateQueries({ queryKey: ['posts', variables.id] })
    queryClient.invalidateQueries({ queryKey: ['posts', 'list', variables.category] })
  },
})
```

## Pattern: Invalidate or Update Directly

```tsx
// Option 1: Invalidate and refetch
onSuccess: () => {
  queryClient.invalidateQueries({ queryKey: ['todos'] })
}

// Option 2: Update cache directly (no network request)
onSuccess: (newTodo) => {
  queryClient.setQueryData(['todos'], (old: Todo[]) => [...old, newTodo])
}

// Option 3: Hybrid - update one, invalidate others
onSuccess: (newTodo) => {
  // Immediately add to list
  queryClient.setQueryData(['todos', 'list'], (old: Todo[]) => [...old, newTodo])
  // Invalidate counts/summaries for eventual consistency
  queryClient.invalidateQueries({ queryKey: ['todos', 'count'] })
}
```

## Context

- Place invalidation in `onSuccess` for successful mutations
- Use `onSettled` if you want to invalidate regardless of success/failure
- Think about all UI surfaces that display related data
- For complex relationships, consider a centralized invalidation helper
- Using hierarchical query keys makes this easier (see `qk-hierarchical-organization`)
