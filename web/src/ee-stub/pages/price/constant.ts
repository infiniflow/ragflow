/**
 * Enterprise seam for the plan names. The enterprise build replaces this file
 * with the pricing catalogue; open source only needs the plan identifiers that
 * shared billing-gated code compares against.
 */

export enum PriceName {
  Trial = 'Trial',
  Starter = 'Starter',
  Pro = 'Pro',
  Enterprise = 'Enterprise',
}

export const PriceNameMapValue = {
  [PriceName.Trial]: 0,
  [PriceName.Starter]: 1,
  [PriceName.Pro]: 2,
  [PriceName.Enterprise]: 3,
};
