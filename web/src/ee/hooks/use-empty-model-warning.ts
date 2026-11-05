/**
 * Enterprise seam for the empty-model warning. Open source has no admin
 * approval flow, so the upstream modal is always the right outcome.
 */
export function useEmptyModelWarning() {
  return {
    isChecking: false,
    showWarning: () => false,
  };
}
