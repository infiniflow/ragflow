# cache-placeholder-vs-initial: Understand Placeholder vs Initial Data

## Priority: MEDIUM

## Explanation

`placeholderData` and `initialData` both provide data before the fetch completes, but behave differently. `initialData` is treated as real cached data, while `placeholderData` is temporary and doesn't persist to cache. Choose based on whether your fallback data should be cached.

## Bad Example

```tsx
// Using initialData when you don't want it cached
function PostPreview({ postId, previewData }: Props) {
  const { data } = useQuery({
    queryKey: ['posts', postId],
    queryFn: () => fetchPost(postId),
    initialData: previewData,  // Wrong: this becomes cached "truth"
    // If previewData is incomplete, it pollutes the cache
    // staleTime applies to this data as if it were fetched
  })
}

// Using placeholderData when you want persistence
function UserProfile({ userId }: Props) {
  const { data } = useQuery({
    queryKey: ['users', userId],
    queryFn: () => fetchUser(userId),
    placeholderData: cachedUserFromList,  // Wrong: won't persist
    // User navigates away and back - placeholder shown again
    // No cache entry created until fetch completes
  })
}
```

## Good Example: placeholderData for Temporary Display

```tsx
// Show list data while fetching detail
function PostDetail({ postId }: { postId: string }) {
  const queryClient = useQueryClient()

  const { data, isPlaceholderData } = useQuery({
    queryKey: ['posts', postId],
    queryFn: () => fetchPost(postId),
    placeholderData: () => {
      // Use partial data from list cache as placeholder
      const posts = queryClient.getQueryData<Post[]>(['posts'])
      return posts?.find(p => p.id === postId)
    },
  })

  return (
    <article className={isPlaceholderData ? 'opacity-50' : ''}>
      <h1>{data?.title}</h1>
      {isPlaceholderData ? (
        <p>Loading full content...</p>
      ) : (
        <div>{data?.content}</div>
      )}
    </article>
  )
}
```

## Good Example: initialData for Known Good Data

```tsx
// SSR: Data fetched on server should be initial
function PostPage({ serverData }: { serverData: Post }) {
  const { data } = useQuery({
    queryKey: ['posts', serverData.id],
    queryFn: () => fetchPost(serverData.id),
    initialData: serverData,
    // Specify when this data was fetched for proper stale calculation
    initialDataUpdatedAt: serverData.fetchedAt,
  })

  return <PostContent post={data} />
}

// Pre-seeding cache with complete data
function App() {
  const queryClient = useQueryClient()

  // If you have complete, authoritative data
  useEffect(() => {
    queryClient.setQueryData(['config'], completeConfigData)
  }, [])
}
```

## Good Example: keepPreviousData Pattern

```tsx
// Keep showing old data while fetching new (pagination, filters)
function ProductList({ page }: { page: number }) {
  const { data, isPlaceholderData } = useQuery({
    queryKey: ['products', page],
    queryFn: () => fetchProducts(page),
    placeholderData: keepPreviousData,  // Built-in helper
  })

  return (
    <div className={isPlaceholderData ? 'opacity-70' : ''}>
      {data?.map(product => (
        <ProductCard key={product.id} product={product} />
      ))}
      {isPlaceholderData && <LoadingOverlay />}
    </div>
  )
}
```

## Comparison Table

| Behavior | `initialData` | `placeholderData` |
|----------|---------------|-------------------|
| Persisted to cache | Yes | No |
| `staleTime` applies | Yes | No (always fetches) |
| `isPlaceholderData` | `false` | `true` |
| Shown to other components | Yes (cached) | No |
| Use case | SSR, complete known data | Preview, previous page |
| Affects `dataUpdatedAt` | Yes (use `initialDataUpdatedAt`) | No |

## Good Example: Combining Both

```tsx
function PostDetail({ postId, ssrData }: Props) {
  const queryClient = useQueryClient()

  const { data } = useQuery({
    queryKey: ['posts', postId],
    queryFn: () => fetchPost(postId),

    // If we have SSR data, use as initial (cached)
    initialData: ssrData,
    initialDataUpdatedAt: ssrData?.fetchedAt,

    // If no SSR data, try to use list preview as placeholder
    placeholderData: () => {
      if (ssrData) return undefined  // Already have initial
      const posts = queryClient.getQueryData<Post[]>(['posts'])
      return posts?.find(p => p.id === postId)
    },
  })
}
```

## Context

- `placeholderData` can be a value or function (lazy evaluation)
- `initialData` affects cache immediately on query creation
- Use `initialDataUpdatedAt` with `initialData` for proper stale calculations
- `keepPreviousData` is a built-in placeholder strategy
- Check `isPlaceholderData` to show loading indicators
- `placeholderData` is ideal for "instant" UI while fetching
