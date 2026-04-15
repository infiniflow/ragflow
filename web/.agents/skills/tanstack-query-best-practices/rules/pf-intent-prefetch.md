# pf-intent-prefetch: Prefetch on User Intent (Hover, Focus)

## Priority: MEDIUM

## Explanation

Prefetch data when users show intent to navigate (hover, focus) rather than waiting for click. This eliminates perceived loading time for likely next actions.

## Bad Example

```tsx
// No prefetching - data fetches on click
function PostList({ posts }: { posts: Post[] }) {
  return (
    <ul>
      {posts.map(post => (
        <li key={post.id}>
          <Link to={`/posts/${post.id}`}>
            {post.title}
          </Link>
          {/* User clicks, waits for data to load */}
        </li>
      ))}
    </ul>
  )
}
```

## Good Example

```tsx
import { useQueryClient } from '@tanstack/react-query'
import { postQueries } from '@/lib/queries'

function PostList({ posts }: { posts: Post[] }) {
  const queryClient = useQueryClient()

  const handlePrefetch = (postId: number) => {
    queryClient.prefetchQuery({
      ...postQueries.detail(postId),
      staleTime: 60 * 1000,  // Consider fresh for 1 minute
    })
  }

  return (
    <ul>
      {posts.map(post => (
        <li key={post.id}>
          <Link
            to={`/posts/${post.id}`}
            onMouseEnter={() => handlePrefetch(post.id)}
            onFocus={() => handlePrefetch(post.id)}
          >
            {post.title}
          </Link>
        </li>
      ))}
    </ul>
  )
}
```

## Good Example: With TanStack Router

```tsx
import { Link } from '@tanstack/react-router'

// TanStack Router has built-in prefetching
function PostList({ posts }: { posts: Post[] }) {
  return (
    <ul>
      {posts.map(post => (
        <li key={post.id}>
          <Link
            to="/posts/$postId"
            params={{ postId: post.id }}
            preload="intent"  // Prefetch on hover/focus
          >
            {post.title}
          </Link>
        </li>
      ))}
    </ul>
  )
}

// Or set as router default
const router = createRouter({
  routeTree,
  defaultPreload: 'intent',
  defaultPreloadDelay: 100,  // Wait 100ms before prefetching
})
```

## Good Example: Prefetch with Delay

```tsx
function PostLink({ post }: { post: Post }) {
  const queryClient = useQueryClient()
  const timeoutRef = useRef<NodeJS.Timeout>()

  const handleMouseEnter = () => {
    // Delay prefetch to avoid unnecessary requests on quick mouse movements
    timeoutRef.current = setTimeout(() => {
      queryClient.prefetchQuery(postQueries.detail(post.id))
    }, 100)
  }

  const handleMouseLeave = () => {
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current)
    }
  }

  return (
    <Link
      to={`/posts/${post.id}`}
      onMouseEnter={handleMouseEnter}
      onMouseLeave={handleMouseLeave}
    >
      {post.title}
    </Link>
  )
}
```

## Prefetch Triggers

| Trigger | When to Use |
|---------|-------------|
| `onMouseEnter` | Desktop, links/buttons user will likely click |
| `onFocus` | Keyboard navigation, accessibility |
| `onTouchStart` | Mobile, before navigation |
| Component mount | Likely next pages, wizard steps |
| Intersection Observer | Below-fold content |

## Context

- Set appropriate `staleTime` when prefetching to avoid immediate refetch
- Consider mobile where hover isn't available
- Don't prefetch everything - focus on likely paths
- Prefetched data uses `gcTime` for retention
- Watch network tab to verify prefetch timing
