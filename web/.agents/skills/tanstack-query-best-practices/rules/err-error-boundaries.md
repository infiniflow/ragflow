# err-error-boundaries: Use Error Boundaries with useQueryErrorResetBoundary

## Priority: HIGH

## Explanation

When using Suspense with TanStack Query, errors propagate to error boundaries. Use `useQueryErrorResetBoundary` to reset query errors when users retry, preventing stuck error states.

## Bad Example

```tsx
// Error boundary without query reset - retry may not work
function ErrorBoundary({ children }: { children: React.ReactNode }) {
  return (
    <ReactErrorBoundary
      fallbackRender={({ error, resetErrorBoundary }) => (
        <div>
          <p>Error: {error.message}</p>
          <button onClick={resetErrorBoundary}>Try again</button>
          {/* resetErrorBoundary alone doesn't reset query state */}
        </div>
      )}
    >
      {children}
    </ReactErrorBoundary>
  )
}

// Query error persists after retry click
```

## Good Example

```tsx
import { useQueryErrorResetBoundary } from '@tanstack/react-query'
import { ErrorBoundary } from 'react-error-boundary'

function QueryErrorBoundary({ children }: { children: React.ReactNode }) {
  const { reset } = useQueryErrorResetBoundary()

  return (
    <ErrorBoundary
      onReset={reset}
      fallbackRender={({ error, resetErrorBoundary }) => (
        <div className="error-container">
          <h2>Something went wrong</h2>
          <pre>{error.message}</pre>
          <button onClick={resetErrorBoundary}>
            Try again
          </button>
        </div>
      )}
    >
      {children}
    </ErrorBoundary>
  )
}

// Usage with Suspense
function App() {
  return (
    <QueryErrorBoundary>
      <Suspense fallback={<Loading />}>
        <Posts />
      </Suspense>
    </QueryErrorBoundary>
  )
}

function Posts() {
  // useSuspenseQuery throws on error, caught by boundary
  const { data } = useSuspenseQuery({
    queryKey: ['posts'],
    queryFn: fetchPosts,
  })

  return <PostList posts={data} />
}
```

## Good Example: With TanStack Router

```tsx
// Route-level error handling
import { createFileRoute } from '@tanstack/react-router'
import { useQueryErrorResetBoundary } from '@tanstack/react-query'

export const Route = createFileRoute('/posts')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(postQueries.list()),

  errorComponent: ({ error, reset }) => {
    const { reset: resetQuery } = useQueryErrorResetBoundary()

    return (
      <div>
        <p>Failed to load posts: {error.message}</p>
        <button
          onClick={() => {
            resetQuery()
            reset()
          }}
        >
          Retry
        </button>
      </div>
    )
  },

  component: PostsPage,
})
```

## Error Boundary Placement Strategy

```tsx
// Granular error boundaries for isolated failures
function Dashboard() {
  return (
    <div className="dashboard">
      {/* Each section can fail independently */}
      <QueryErrorBoundary>
        <Suspense fallback={<Skeleton />}>
          <RecentActivity />
        </Suspense>
      </QueryErrorBoundary>

      <QueryErrorBoundary>
        <Suspense fallback={<Skeleton />}>
          <Statistics />
        </Suspense>
      </QueryErrorBoundary>

      <QueryErrorBoundary>
        <Suspense fallback={<Skeleton />}>
          <Notifications />
        </Suspense>
      </QueryErrorBoundary>
    </div>
  )
}
```

## Context

- `useQueryErrorResetBoundary` clears error state for all queries in the boundary
- Always pair Suspense queries with error boundaries
- Place boundaries based on failure isolation needs
- Consider inline error handling for non-critical data
- The reset only affects queries that were in error state
