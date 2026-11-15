/**
 * Enterprise seam for the billing plan requests. The enterprise build replaces
 * this file with the price catalogue requests; open source has no billing, so
 * the current plan is empty.
 */

export const useFetchCurrentPlan = () => ({
  data: { plan_name: '' },
  loading: false,
});

export const useFetchPlanList = () => ({
  data: [],
  loading: false,
});

export const useCharge = () => ({
  charge: async (_params: unknown) => {},
  loading: false,
});

export const useCancelPlan = () => ({
  cancelPlan: async (_params?: unknown) => {},
  loading: false,
});

export const useDirectPaymentConvergence = () => {};

export const useHandleTrialUpgradeSetupRetry = (_status: string | null) => {};

export const useSetupIntentPayment = () => ({
  setupIntentPayment: async (_params?: unknown) => {},
  loading: false,
});

export const getNextMonth = () => '';
