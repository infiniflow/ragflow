import { timingSafeEqual } from 'node:crypto';

export function isAuthorized(req, authToken) {
  const expectedToken = String(authToken || '').trim();
  if (!expectedToken) {
    return false;
  }

  const authorization = Buffer.from(String(req.headers.authorization || '').trim());
  const expectedAuthorization = Buffer.from(`Bearer ${expectedToken}`);
  return authorization.length === expectedAuthorization.length
    && timingSafeEqual(authorization, expectedAuthorization);
}
