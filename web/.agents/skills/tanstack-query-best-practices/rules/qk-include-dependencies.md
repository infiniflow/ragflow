# qk-include-dependencies: Include All Variables the Query Depends On

## Priority: CRITICAL

## Explanation

If your query function depends on a variable, that variable must be included in the query key. This ensures independent caching per variable combination and automatic refetching when dependencies change. Missing dependencies cause stale data bugs and cache collisions.

## Bad Example

```tsx
function UserPosts({ userId }: { userId: string }) {
  // Missing userId in query key - all users share the same cache!
  const { data } = useQuery({
    queryKey: ['posts'],
    queryFn: () => fetchPostsByUser(userId),
  })

  return <PostList posts={data} />
}

function FilteredTodos({ status, page }: { status: string; page: number }) {
  // Missing filter parameters - won't refetch when filters change
  const { data } = useQuery({
    queryKey: ['todos'],
    queryFn: () => fetchTodos({ status, page }),
  })

  return <TodoList todos={data} />
}
```

## Good Example

```tsx
function UserPosts({ userId }: { userId: string }) {
  // userId included - each user has their own cache entry
  const { data } = useQuery({
    queryKey: ['posts', userId],
    queryFn: () => fetchPostsByUser(userId),
  })

  return <PostList posts={data} />
}

function FilteredTodos({ status, page }: { status: string; page: number }) {
  // All dependencies included - refetches when any change
  const { data } = useQuery({
    queryKey: ['todos', { status, page }],
    queryFn: () => fetchTodos({ status, page }),
  })

  return <TodoList todos={data} />
}
```

## Context

- This is arguably the most important query key rule
- Applies whenever query function uses external variables
- Prevents subtle bugs where different contexts share cached data
- Works in conjunction with staleTime - even with long staleTime, changing keys triggers new fetches
