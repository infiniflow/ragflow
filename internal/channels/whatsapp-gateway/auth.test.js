import assert from 'node:assert/strict';
import test from 'node:test';

import { isAuthorized } from './auth.js';

test('rejects control requests when the gateway token is unset', () => {
  assert.equal(isAuthorized({ headers: {} }, ''), false);
  assert.equal(isAuthorized({ headers: { authorization: 'Bearer anything' } }, ''), false);
});

test('requires the configured bearer token for control requests', () => {
  assert.equal(isAuthorized({ headers: {} }, 'gateway-secret'), false);
  assert.equal(
    isAuthorized({ headers: { authorization: 'Bearer wrong' } }, 'gateway-secret'),
    false,
  );
  assert.equal(
    isAuthorized({ headers: { authorization: 'Bearer gateway-secret' } }, 'gateway-secret'),
    true,
  );
});
