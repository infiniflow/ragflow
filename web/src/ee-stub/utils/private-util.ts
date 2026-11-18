/**
 * Enterprise seam for the navigation helpers used by auth guards and the
 * request interceptors.
 *
 * The enterprise build sends users back to login with a `redirect` query so
 * they land on the page they were viewing. Open source keeps the upstream
 * redirect target and has no post-login redirect hop.
 */

export { redirectToLogin } from '@/utils/authorization-util';

export function redirectToSpecifiedPage(page: string) {
  window.location.href = location.origin + page;
}
