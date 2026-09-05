import { canAccessUserSettingsPath } from './settings-access-policy';

describe('settings access policy', () => {
  it.each([
    '/user-setting/profile',
    '/user-setting/profile/',
    '/user-setting/model',
    '/user-setting/model/',
  ])('allows a regular user to open %s', (path) => {
    expect(canAccessUserSettingsPath(path, false)).toBe(true);
  });

  it.each([
    '/user-setting',
    '/user-setting/team',
    '/user-setting/api',
    '/user-setting/mcp',
    '/user-setting/data-source',
    '/user-setting/chat-channel',
    '/user-setting/data-source/data-source-detail-page',
    '/user-setting/profile-extra',
    '/user-setting/model/private',
    '/',
  ])('denies a regular user access to %s', (path) => {
    expect(canAccessUserSettingsPath(path, false)).toBe(false);
  });

  it('fails closed for an admin-only child route', () => {
    expect(
      canAccessUserSettingsPath('/user-setting/profile', false, true),
    ).toBe(false);
  });

  it.each([
    '/user-setting',
    '/user-setting/team',
    '/user-setting/api',
    '/user-setting/mcp',
    '/user-setting/data-source',
    '/user-setting/chat-channel',
    '/user-setting/data-source/data-source-detail-page',
  ])('allows a superuser to open %s', (path) => {
    expect(canAccessUserSettingsPath(path, true)).toBe(true);
  });
});
