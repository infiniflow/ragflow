/**
 * Enterprise seam for the admin system variables. The enterprise build
 * replaces this file with the request that reads and writes them; open source
 * has no system-variable admin UI, so the values stay empty except for the
 * whitelist flag that keeps the open source whitelist page reachable.
 */
const openSourceVariables: Record<string, { value?: unknown } | undefined> = {
  enable_whitelist: { value: true },
};

export default function useAdminVariables() {
  return {
    variables: openSourceVariables,
    isFetching: false,
    refetch: async () => {},
    setVariables: async (_variables: Record<string, unknown>) => {},
    isUpdating: false,
  };
}
