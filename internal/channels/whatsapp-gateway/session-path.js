import path from 'node:path';

function isSafeSessionKey(sessionKey) {
  return (
    typeof sessionKey === 'string' &&
    sessionKey.length > 0 &&
    sessionKey !== '.' &&
    sessionKey !== '..' &&
    !sessionKey.includes('/') &&
    !sessionKey.includes('\\') &&
    !sessionKey.includes('\0')
  );
}

export function decodeSessionKey(encodedKey) {
  let sessionKey;
  try {
    sessionKey = decodeURIComponent(encodedKey);
  } catch {
    return null;
  }

  sessionKey = sessionKey.trim();
  return isSafeSessionKey(sessionKey) ? sessionKey : null;
}

export function resolveSessionDir(dataDir, sessionKey) {
  if (!isSafeSessionKey(sessionKey)) {
    throw new TypeError('invalid session key');
  }

  const root = path.resolve(dataDir);
  const sessionDir = path.resolve(root, sessionKey);
  const relative = path.relative(root, sessionDir);
  if (!relative || relative === '..' || relative.startsWith(`..${path.sep}`) || path.isAbsolute(relative)) {
    throw new TypeError('session directory escapes the data directory');
  }
  return sessionDir;
}
