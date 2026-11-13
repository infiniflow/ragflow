/**
 * Enterprise seam for the runtime billing status lookup.
 *
 * The enterprise build replaces this file with a request to the billing
 * endpoint so the header and price pages can gate on the server-side flag.
 * Open source has no billing: the status is always disabled and the boot
 * sequence does not wait on a request.
 */

export const fetchBillingStatus = async (): Promise<boolean> => false;

export const isBillingEnabled = (): boolean | null => false;
