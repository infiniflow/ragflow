# ssr-dehydration: Use Dehydrate/Hydrate Pattern for SSR

## Priority: MEDIUM

## Explanation

For server-side rendering, prefetch queries on the server, dehydrate the cache to a serializable format, send it to the client, and hydrate on the client. This prevents content flash and duplicate requests.

## Bad Example

```tsx
// No SSR data passing - client refetches everything
// server-side
export async function getServerSideProps() {
  const data = await fetchPosts()
  return { props: { posts: data } }  // Bypasses React Query cache
}

// client-side
function PostsPage({ posts }: { posts: Post[] }) {
  // This doesn't benefit from the server fetch
  const { data } = useQuery({
    queryKey: ['posts'],
    queryFn: fetchPosts,
    // Will refetch on client, causing flash
  })

  return <PostList posts={data ?? posts} />  // Awkward fallback pattern
}
```

## Good Example: Next.js App Router

```tsx
// app/posts/page.tsx
import {
  dehydrate,
  HydrationBoundary,
  QueryClient,
} from '@tanstack/react-query'
import { postQueries } from '@/lib/queries'

export default async function PostsPage() {
  const queryClient = new QueryClient()

  await queryClient.prefetchQuery(postQueries.list())

  return (
    <HydrationBoundary state={dehydrate(queryClient)}>
      <PostList />
    </HydrationBoundary>
  )
}

// components/PostList.tsx
'use client'

import { useSuspenseQuery } from '@tanstack/react-query'
import { postQueries } from '@/lib/queries'

export function PostList() {
  const { data: posts } = useSuspenseQuery(postQueries.list())

  return (
    <ul>
      {posts.map(post => (
        <li key={post.id}>{post.title}</li>
      ))}
    </ul>
  )
}
```

## Good Example: TanStack Start/Router

```tsx
// routes/posts.tsx
import { createFileRoute } from '@tanstack/react-router'
import { postQueries } from '@/lib/queries'

export const Route = createFileRoute('/posts')({
  loader: async ({ context: { queryClient } }) => {
    // Prefetch in route loader
    await queryClient.ensureQueryData(postQueries.list())
  },
  component: PostsPage,
})

function PostsPage() {
  const { data: posts } = useSuspenseQuery(postQueries.list())
  return <PostList posts={posts} />
}
```

## Good Example: Manual SSR Setup

```tsx
// server.tsx
import { dehydrate, QueryClient } from '@tanstack/react-query'
import { renderToString } from 'react-dom/server'

export async function render(url: string) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 60 * 1000,  // Prevent immediate client refetch
      },
    },
  })

  // Prefetch required data
  await queryClient.prefetchQuery({
    queryKey: ['posts'],
    queryFn: fetchPosts,
  })

  const dehydratedState = dehydrate(queryClient)

  const html = renderToString(
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>
  )

  // Serialize safely - JSON.stringify is XSS vulnerable
  const serializedState = serialize(dehydratedState)

  return `
    <html>
      <body>
        <div id="app">${html}</div>
        <script>window.__DEHYDRATED_STATE__ = ${serializedState}</script>
      </body>
    </html>
  `
}

// client.tsx
import { hydrate, QueryClient, QueryClientProvider } from '@tanstack/react-query'

const queryClient = new QueryClient()
hydrate(queryClient, window.__DEHYDRATED_STATE__)

hydrateRoot(
  document.getElementById('app'),
  <QueryClientProvider client={queryClient}>
    <App />
  </QueryClientProvider>
)
```

## Context

- Create new QueryClient per request to prevent data sharing between users
- Set `staleTime > 0` on server to prevent immediate client refetch
- Use a safe serializer (not JSON.stringify) to prevent XSS
- Failed queries aren't dehydrated by default; use `shouldDehydrateQuery` to override
- `HydrationBoundary` can be nested for route-level prefetching
