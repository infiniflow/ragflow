# perf-select-transform: Use Select to Transform and Filter Data

## Priority: LOW

## Explanation

The `select` option transforms query data before it reaches your component. Use it for filtering, sorting, or deriving data. Benefits include memoization (re-runs only when data changes) and reduced component re-renders.

## Bad Example

```tsx
// Transforming in component - runs on every render
function CompletedTodos() {
  const { data: todos } = useQuery({
    queryKey: ['todos'],
    queryFn: fetchTodos,
  })

  // This filtering runs on every render
  const completedTodos = todos?.filter(todo => todo.completed) ?? []
  const sortedTodos = [...completedTodos].sort((a, b) =>
    new Date(b.completedAt).getTime() - new Date(a.completedAt).getTime()
  )

  return <TodoList todos={sortedTodos} />
}
```

## Good Example

```tsx
// Using select - runs only when data changes
function CompletedTodos() {
  const { data: completedTodos } = useQuery({
    queryKey: ['todos'],
    queryFn: fetchTodos,
    select: (todos) =>
      todos
        .filter(todo => todo.completed)
        .sort((a, b) =>
          new Date(b.completedAt).getTime() - new Date(a.completedAt).getTime()
        ),
  })

  return <TodoList todos={completedTodos ?? []} />
}
```

## Good Example: Selecting Specific Fields

```tsx
// Derive computed values
function TodoStats() {
  const { data: stats } = useQuery({
    queryKey: ['todos'],
    queryFn: fetchTodos,
    select: (todos) => ({
      total: todos.length,
      completed: todos.filter(t => t.completed).length,
      pending: todos.filter(t => !t.completed).length,
      completionRate: todos.length
        ? (todos.filter(t => t.completed).length / todos.length) * 100
        : 0,
    }),
  })

  return (
    <div>
      <span>{stats?.completed} / {stats?.total} completed</span>
      <span>({stats?.completionRate.toFixed(1)}%)</span>
    </div>
  )
}
```

## Good Example: Stable Select with useCallback

```tsx
// When select depends on external values, stabilize with useCallback
function FilteredTodos({ status }: { status: 'all' | 'active' | 'completed' }) {
  const selectTodos = useCallback(
    (todos: Todo[]) => {
      switch (status) {
        case 'active':
          return todos.filter(t => !t.completed)
        case 'completed':
          return todos.filter(t => t.completed)
        default:
          return todos
      }
    },
    [status]
  )

  const { data: filteredTodos } = useQuery({
    queryKey: ['todos'],
    queryFn: fetchTodos,
    select: selectTodos,
  })

  return <TodoList todos={filteredTodos ?? []} />
}
```

## Good Example: Picking Single Item from List

```tsx
// Select single item from cached list
function useTodoById(id: number) {
  return useQuery({
    queryKey: ['todos'],
    queryFn: fetchTodos,
    select: (todos) => todos.find(todo => todo.id === id),
  })
}

// Usage - shares cache with list query
function TodoDetail({ id }: { id: number }) {
  const { data: todo } = useTodoById(id)

  if (!todo) return <div>Todo not found</div>
  return <div>{todo.title}</div>
}
```

## When to Use Select

| Scenario | Use Select? |
|----------|-------------|
| Filtering list data | Yes |
| Sorting data | Yes |
| Computing derived values | Yes |
| Picking single item from list | Yes |
| Heavy transformations | Yes (memoized) |
| Simple data pass-through | No |
| Transformation needs external state | Yes, with useCallback |

## Context

- `select` leverages structural sharing - only re-runs when data actually changes
- Original query data stays cached; transformation applies to consumer
- Multiple components can use different `select` on the same query
- Avoid unstable function references - use `useCallback` when needed
- For complex transformations, consider useMemo in component instead if readability suffers
