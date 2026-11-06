export function isAuthorized(req, authToken) {
  const expectedToken = String(authToken || '').trim();
  if (!expectedToken) {
    return false;
  }

  const authorization = String(req.headers.authorization || '').trim();
  return authorization === `Bearer ${expectedToken}`;
}
