# cache-gc-time: Configure gcTime for Inactive Query Retention

## Priority: CRITICAL

## Explanation

`gcTime` (garbage collection time, formerly `cacheTime`) controls how long inactive queries remain in the cache before being garbage collected. Default is 5 minutes. Configure based on your navigation patterns and memory constraints.

## Bad Example

```tsx
// Not considering gcTime for frequently revisited pages
const { data } = useQuery({
  queryKey: ['dashboard-stats'],
  queryFn: fetchDashboardStats,
  // Default gcTime of 5 minutes - might be too short for frequently revisited data
})

// Setting gcTime too high without consideration
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      gcTime: Infinity,  // Never garbage collect - potential memory leak
    },
  },
})

// Setting gcTime to 0 - cache is immediately removed
const { data } = useQuery({
  queryKey: ['user-data'],
  queryFn: fetchUserData,
  gcTime: 0,  // Loses cache benefits entirely
})
```

## Good Example

```tsx
// Longer gcTime for frequently revisited data
const { data } = useQuery({
  queryKey: ['dashboard-stats'],
  queryFn: fetchDashboardStats,
  gcTime: 30 * 60 * 1000,  // 30 minutes - user returns to dashboard often
})

// Shorter gcTime for rarely revisited large data
const { data: report } = useQuery({
  queryKey: ['detailed-report', reportId],
  queryFn: () => fetchReport(reportId),
  gcTime: 2 * 60 * 1000,  // 2 minutes - large payload, viewed once
})

// Sensible default with query-specific overrides
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      gcTime: 10 * 60 * 1000,  // 10 minutes default
    },
  },
})
```

## Understanding gcTime vs staleTime

```
Query Mount → Data Fresh (staleTime) → Data Stale → Query Unmount → gcTime countdown → Garbage Collected

Timeline example (staleTime: 1min, gcTime: 5min):
0:00 - Query mounts, fetches data
0:00-1:00 - Data is fresh (no background refetch)
1:00+ - Data is stale (background refetch on new mount)
5:00 - User navigates away, query unmounts
5:00-10:00 - Data in cache but inactive (gcTime countdown)
10:00 - Data garbage collected (next mount = full loading state)
```

## Recommended gcTime Values

| Scenario | gcTime | Rationale |
|----------|--------|-----------|
| Frequently revisited routes | 15 - 30min | Instant navigation |
| Detail pages (viewed once) | 2 - 5min | Memory efficient |
| Large payloads | 1 - 2min | Prevent memory bloat |
| Critical user data | 30min+ | Offline-like experience |
| SSR hydration | >= 2s | Prevent hydration issues |

## Context

- gcTime countdown starts when ALL query observers unmount
- Remounting before gcTime expires returns cached data instantly
- Setting gcTime < staleTime is rarely useful
- For SSR, avoid gcTime: 0 (use minimum 2000ms to allow hydration)
- Monitor memory usage in long-running applications
