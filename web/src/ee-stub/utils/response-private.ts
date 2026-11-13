/**
 * Enterprise seam for response-error handling.
 *
 * The enterprise build inspects the response code to open the billing
 * upgrade modal and to keep license-related login errors quiet, then returns
 * `true` so the shared interceptor skips its generic notification.
 * Open source has neither, so every error keeps the upstream notification.
 */

export type ResponseErrorContext = {
  url?: string;
};

export const handleEnterpriseResponseError = async (
  _data: { code?: number; message?: string } | undefined,
  _context?: ResponseErrorContext,
): Promise<boolean> => false;
