/**
 * Backend runtime language detection.
 *
 * Fetches /api/v1/language at runtime (once, then caches) so the front end
 * can choose between Go-specific and Python-specific code paths without a
 * build-time flag.
 *
 * Pattern: same lightweight fetch-once approach as enterprise billingStatus.ts.
 */

let backendLanguage: string | null = null;
let fetching: Promise<string> | null = null;

export const fetchBackendLanguage = async (): Promise<string> => {
  if (backendLanguage) return backendLanguage;
  if (fetching) return fetching;

  fetching = (async () => {
    try {
      const res = await fetch('/api/v1/language');
      const body = await res.json();
      backendLanguage = body.data?.language === 'go' ? 'go' : 'python';
      return backendLanguage;
    } catch {
      backendLanguage = 'python';
      return 'python';
    }
  })();

  return fetching;
};

export const getBackendLanguage = (): string | null => backendLanguage;
