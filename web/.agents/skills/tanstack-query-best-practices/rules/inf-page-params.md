# inf-page-params: Always Provide getNextPageParam for Infinite Queries

## Priority: MEDIUM

## Explanation

`useInfiniteQuery` requires `getNextPageParam` to determine how to fetch subsequent pages. This function receives the last page's data and must return the next page parameter, or `undefined` when there are no more pages.

## Bad Example

```tsx
// Missing getNextPageParam - can't load more pages
const { data, fetchNextPage } = useInfiniteQuery({
  queryKey: ['posts'],
  queryFn: ({ pageParam }) => fetchPosts(pageParam),
  initialPageParam: 1,
  // Missing getNextPageParam - fetchNextPage won't work correctly
})
```

## Good Example: Offset-Based Pagination

```tsx
const {
  data,
  fetchNextPage,
  hasNextPage,
  isFetchingNextPage,
} = useInfiniteQuery({
  queryKey: ['posts'],
  queryFn: ({ pageParam }) => fetchPosts({ page: pageParam, limit: 20 }),
  initialPageParam: 1,
  getNextPageParam: (lastPage, allPages) => {
    // Return next page number, or undefined if no more pages
    if (lastPage.length < 20) {
      return undefined  // No more pages
    }
    return allPages.length + 1
  },
})
```

## Good Example: Cursor-Based Pagination

```tsx
interface PostsResponse {
  posts: Post[]
  nextCursor: string | null
}

const { data, fetchNextPage, hasNextPage } = useInfiniteQuery({
  queryKey: ['posts'],
  queryFn: ({ pageParam }): Promise<PostsResponse> =>
    fetchPosts({ cursor: pageParam }),
  initialPageParam: undefined as string | undefined,
  getNextPageParam: (lastPage) => lastPage.nextCursor ?? undefined,
})
```

## Good Example: Bi-directional Pagination

```tsx
const { data, fetchNextPage, fetchPreviousPage, hasNextPage, hasPreviousPage } =
  useInfiniteQuery({
    queryKey: ['messages', chatId],
    queryFn: ({ pageParam }) => fetchMessages({ chatId, cursor: pageParam }),
    initialPageParam: { direction: 'initial' } as PageParam,
    getNextPageParam: (lastPage) =>
      lastPage.hasMore ? { cursor: lastPage.nextCursor, direction: 'next' } : undefined,
    getPreviousPageParam: (firstPage) =>
      firstPage.hasPrevious
        ? { cursor: firstPage.prevCursor, direction: 'prev' }
        : undefined,
  })
```

## Good Example: With Total Count

```tsx
interface PaginatedResponse<T> {
  items: T[]
  total: number
  page: number
  pageSize: number
}

const { data, hasNextPage } = useInfiniteQuery({
  queryKey: ['products', filters],
  queryFn: ({ pageParam }) =>
    fetchProducts({ ...filters, page: pageParam, pageSize: 20 }),
  initialPageParam: 1,
  getNextPageParam: (lastPage) => {
    const totalPages = Math.ceil(lastPage.total / lastPage.pageSize)
    if (lastPage.page < totalPages) {
      return lastPage.page + 1
    }
    return undefined
  },
})
```

## Accessing Flattened Data

```tsx
// data.pages is an array of page responses
// Flatten for easier iteration
const allPosts = data?.pages.flatMap(page => page.posts) ?? []

return (
  <div>
    {allPosts.map(post => (
      <PostCard key={post.id} post={post} />
    ))}
    {hasNextPage && (
      <button
        onClick={() => fetchNextPage()}
        disabled={isFetchingNextPage}
      >
        {isFetchingNextPage ? 'Loading...' : 'Load More'}
      </button>
    )}
  </div>
)
```

## Context

- `getNextPageParam` returning `undefined` sets `hasNextPage` to `false`
- For bi-directional scrolling, also provide `getPreviousPageParam`
- `initialPageParam` is required and sets the first page parameter
- Use `maxPages` option to limit stored pages for memory management
- Consider `select` to transform page structure for component consumption
