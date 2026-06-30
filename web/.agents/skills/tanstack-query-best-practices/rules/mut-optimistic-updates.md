# mut-optimistic-updates: Implement Optimistic Updates for Responsive UI

## Priority: HIGH

## Explanation

Optimistic updates immediately reflect changes in the UI before the server confirms them, creating a snappy user experience. Implement them for user-initiated mutations where the expected outcome is predictable.

## Bad Example

```tsx
// No optimistic update - UI waits for server response
const mutation = useMutation({
  mutationFn: toggleTodoComplete,
  onSuccess: () => {
    queryClient.invalidateQueries({ queryKey: ['todos'] })
  },
})

// User clicks checkbox, waits 200-500ms for visual feedback
```

## Good Example: Via Cache Manipulation

```tsx
const mutation = useMutation({
  mutationFn: toggleTodoComplete,
  onMutate: async (todoId) => {
    // 1. Cancel outgoing refetches to prevent overwriting optimistic update
    await queryClient.cancelQueries({ queryKey: ['todos'] })

    // 2. Snapshot previous value for potential rollback
    const previousTodos = queryClient.getQueryData(['todos'])

    // 3. Optimistically update the cache
    queryClient.setQueryData(['todos'], (old: Todo[]) =>
      old.map((todo) =>
        todo.id === todoId ? { ...todo, completed: !todo.completed } : todo
      )
    )

    // 4. Return context for rollback
    return { previousTodos }
  },
  onError: (err, todoId, context) => {
    // Rollback on error
    queryClient.setQueryData(['todos'], context?.previousTodos)
  },
  onSettled: () => {
    // Refetch to ensure consistency regardless of success/failure
    queryClient.invalidateQueries({ queryKey: ['todos'] })
  },
})
```

## Good Example: Via UI Variables (Simpler)

```tsx
// When mutation only affects local UI, use mutation state directly
function TodoItem({ todo }: { todo: Todo }) {
  const mutation = useMutation({
    mutationFn: toggleTodoComplete,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['todos'] })
    },
  })

  // Show optimistic state while pending
  const displayCompleted = mutation.isPending
    ? !todo.completed  // Optimistic: show toggled state
    : todo.completed   // Settled: show actual state

  return (
    <div>
      <input
        type="checkbox"
        checked={displayCompleted}
        disabled={mutation.isPending}
        onChange={() => mutation.mutate(todo.id)}
      />
      <span style={{ opacity: mutation.isPending ? 0.5 : 1 }}>
        {todo.title}
      </span>
    </div>
  )
}
```

## Good Example: Optimistic Create with Temporary ID

```tsx
const createTodo = useMutation({
  mutationFn: (newTodo: CreateTodoInput) => api.createTodo(newTodo),
  onMutate: async (newTodo) => {
    await queryClient.cancelQueries({ queryKey: ['todos'] })
    const previousTodos = queryClient.getQueryData(['todos'])

    // Add with temporary ID
    const optimisticTodo = {
      id: `temp-${Date.now()}`,
      ...newTodo,
      completed: false,
      createdAt: new Date().toISOString(),
    }

    queryClient.setQueryData(['todos'], (old: Todo[]) => [...old, optimisticTodo])

    return { previousTodos, optimisticTodo }
  },
  onError: (err, newTodo, context) => {
    queryClient.setQueryData(['todos'], context?.previousTodos)
  },
  onSuccess: (data, variables, context) => {
    // Replace temp todo with real one
    queryClient.setQueryData(['todos'], (old: Todo[]) =>
      old.map((todo) =>
        todo.id === context?.optimisticTodo.id ? data : todo
      )
    )
  },
})
```

## When to Use Each Approach

| Approach | Use When |
|----------|----------|
| Cache Manipulation | Update appears in multiple places, complex data structures |
| UI Variables | Update only visible in one component, simpler implementation |

## Context

- Always provide rollback logic in `onError`
- Cancel queries before optimistic update to prevent race conditions
- Call `invalidateQueries` in `onSettled` to sync with server truth
- For forms, consider if validation should block optimistic display
- Test error scenarios to verify rollback works correctly
