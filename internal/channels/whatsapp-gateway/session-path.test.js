import assert from 'node:assert/strict';
import path from 'node:path';
import test from 'node:test';

import { decodeSessionKey, resolveSessionDir } from './session-path.js';

test('decodes ordinary session keys', () => {
  assert.equal(decodeSessionKey('tenant-123_user-456'), 'tenant-123_user-456');
  assert.equal(decodeSessionKey('tenant%2D123'), 'tenant-123');
});

test('rejects encoded path separators and malformed escapes', () => {
  const requestURL = new URL('/whatsapp/..%2f..%2foutside/start', 'http://localhost');
  const segment = requestURL.pathname.split('/').filter(Boolean)[1];
  assert.equal(decodeSessionKey(segment), null);
  assert.equal(decodeSessionKey('%5c..%5coutside'), null);
  assert.equal(decodeSessionKey('%E0%A4%A'), null);
});

test('keeps session directories beneath the configured data directory', () => {
  const dataDir = path.resolve('tmp', 'whatsapp-data', 'sessions');
  const sessionDir = resolveSessionDir(dataDir, 'account-123');
  assert.equal(sessionDir, path.join(dataDir, 'account-123'));
  assert.throws(() => resolveSessionDir(dataDir, '../../outside'), /invalid session key/);
  assert.throws(() => resolveSessionDir(dataDir, '..'), /invalid session key/);
});
