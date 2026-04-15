# mut-mutation-state: Use useMutationState for Cross-Component Mutation Tracking

## Priority: MEDIUM

## Explanation

`useMutationState` allows you to access mutation state from anywhere in your component tree, not just where `useMutation` was called. Use it to show loading indicators, display optimistic updates, or track pending mutations across components.

## Bad Example

```tsx
// Prop drilling mutation state
function App() {
  const mutation = useMutation({ mutationFn: createPost })

  return (
    <div>
      <Header isPending={mutation.isPending} />
      <Sidebar isPending={mutation.isPending} />
      <Content mutation={mutation} />
      <Footer isPending={mutation.isPending} />
    </div>
  )
}

// Or using context for every mutation
const MutationContext = createContext<UseMutationResult | null>(null)
```

## Good Example

```tsx
// Define mutation with a key
const useCreatePost = () => useMutation({
  mutationKey: ['create-post'],
  mutationFn: createPost,
})

// In the component that triggers mutation
function CreatePostButton() {
  const mutation = useCreatePost()

  return (
    <button onClick={() => mutation.mutate(newPost)}>
      Create Post
    </button>
  )
}

// In any other component - track mutation state
function GlobalLoadingIndicator() {
  const pendingMutations = useMutationState({
    filters: { status: 'pending' },
    select: (mutation) => mutation.state.variables,
  })

  if (pendingMutations.length === 0) return null

  return (
    <div className="global-loading">
      Saving {pendingMutations.length} item(s)...
    </div>
  )
}
```

## Good Example: Optimistic UI in Separate Component

```tsx
// Mutation defined in form
function TodoForm() {
  const createTodo = useMutation({
    mutationKey: ['create-todo'],
    mutationFn: (todo: NewTodo) => api.createTodo(todo),
  })

  return <form onSubmit={...}>...</form>
}

// Optimistic display in list (different component)
function TodoList() {
  const { data: todos } = useQuery({ queryKey: ['todos'], queryFn: fetchTodos })

  // Get pending todo creations
  const pendingTodos = useMutationState({
    filters: {
      mutationKey: ['create-todo'],
      status: 'pending',
    },
    select: (mutation) => mutation.state.variables as NewTodo,
  })

  return (
    <ul>
      {/* Existing todos */}
      {todos?.map(todo => (
        <TodoItem key={todo.id} todo={todo} />
      ))}

      {/* Optimistic todos (pending creation) */}
      {pendingTodos.map((todo, index) => (
        <TodoItem
          key={`pending-${index}`}
          todo={{ ...todo, id: `temp-${index}` }}
          isPending
        />
      ))}
    </ul>
  )
}
```

## Good Example: Track Specific Mutations

```tsx
function PostActions({ postId }: { postId: string }) {
  // Track if THIS post is being deleted
  const isDeletingThisPost = useMutationState({
    filters: {
      mutationKey: ['delete-post', postId],
      status: 'pending',
    },
    select: () => true,
  }).length > 0

  // Track if THIS post is being updated
  const isUpdatingThisPost = useMutationState({
    filters: {
      mutationKey: ['update-post', postId],
      status: 'pending',
    },
    select: () => true,
  }).length > 0

  return (
    <div>
      <button disabled={isDeletingThisPost || isUpdatingThisPost}>
        {isDeletingThisPost ? 'Deleting...' : 'Delete'}
      </button>
    </div>
  )
}
```

## Filters Reference

```tsx
useMutationState({
  filters: {
    mutationKey: ['key'],           // Match mutation key
    status: 'pending',              // 'idle' | 'pending' | 'success' | 'error'
    predicate: (mutation) => bool,  // Custom filter function
  },
  select: (mutation) => {
    // Transform each matching mutation
    // mutation.state contains: variables, data, error, status, etc.
    return mutation.state.variables
  },
})
```

## Context

- Requires `mutationKey` on mutations you want to track
- Returns array of selected values from matching mutations
- Updates reactively as mutations progress
- Use `status` filter to track pending/success/error states
- Enables optimistic UI without prop drilling
- Pairs with `mutationKey` arrays for granular tracking (e.g., `['delete-post', postId]`)
