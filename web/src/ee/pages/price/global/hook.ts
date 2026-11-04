/**
 * Enterprise seam for the upgrade prompts. The enterprise build replaces this
 * file with the events and modals that route the user to the price page; open
 * source has no billing, so every entry point is a no-op.
 */

export const showUpgradeTipsModal = (_options?: unknown) => {};

export const showPriceConfirmModal = (_options?: unknown) => {};

export const showFreeUpgradeTipsModal = (_options?: unknown) => {};

export const showPriceModal = (_price?: unknown) => {};

export const useShowUpgradeTipsModal = () => ({
  upgradeTips: undefined,
  hideUpgradeTips: () => {},
});

export const useShowFreeUpgradeTipsModal = () => ({
  freeUpgradeTips: undefined,
  hideFreeUpgradeTips: () => {},
});

export const useShowConfirmPriceModal = () => ({
  confirmPrice: undefined,
  hideConfirmPrice: () => {},
});

export const useComputedRouterChangeCount = () => 0;
