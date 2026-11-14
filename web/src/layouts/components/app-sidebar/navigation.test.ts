import { Routes } from '@/routes';
import {
  getActiveIntegrationKey,
  getActiveNavItemKey,
  getActivePageTab,
  matchesPath,
  NavItemKey,
} from './navigation';

describe('matchesPath', () => {
  it('matches exact paths and path-segment boundaries', () => {
    expect(matchesPath('/datasets', '/datasets')).toBe(true);
    expect(matchesPath('/dataset/files/1', '/dataset')).toBe(true);
    expect(matchesPath('/chat/abc', '/chat')).toBe(true);
  });

  it('does not match loose substrings', () => {
    expect(matchesPath('/user-setting/chat-channel', '/chat')).toBe(false);
    expect(matchesPath('/searches', '/search')).toBe(false);
    expect(matchesPath('/datasets-extra', '/datasets')).toBe(false);
  });
});

describe('getActiveNavItemKey', () => {
  it('matches home only on the exact root path', () => {
    expect(getActiveNavItemKey('/')).toBe(NavItemKey.Home);
    expect(getActiveNavItemKey('/datasets')).not.toBe(NavItemKey.Home);
  });

  it.each([
    ['/datasets', NavItemKey.Context],
    ['/dataset/files/1', NavItemKey.Context],
    ['/dataset/retrieval/1', NavItemKey.Context],
    ['/memories', NavItemKey.Context],
    ['/memory/memory-message/1', NavItemKey.Context],
    ['/chats', NavItemKey.Application],
    ['/chat/1', NavItemKey.Application],
    ['/searches', NavItemKey.Application],
    ['/search/1', NavItemKey.Application],
    ['/agents', NavItemKey.Studio],
    ['/agent-templates', NavItemKey.Studio],
    ['/agent/1', NavItemKey.Studio],
    ['/pipeline/1', NavItemKey.Studio],
    ['/agent-log-page/1', NavItemKey.Studio],
    ['/compilation-templates/edit-next', NavItemKey.Studio],
    ['/compilation-templates/edit-next/1', NavItemKey.Studio],
    ['/files', NavItemKey.Library],
    ['/files/skills', NavItemKey.Library],
    ['/team', NavItemKey.Team],
  ])('maps %s to %s', (pathname, expected) => {
    expect(getActiveNavItemKey(pathname)).toBe(expected);
  });

  it('leaves integrations children to the integrations group', () => {
    expect(getActiveNavItemKey('/integrations/model')).toBeUndefined();
    expect(getActiveNavItemKey('/integrations/mcp')).toBeUndefined();
  });

  it('activates nothing for profile and api settings', () => {
    expect(getActiveNavItemKey('/user-setting/profile')).toBeUndefined();
    expect(getActiveNavItemKey('/user-setting/api')).toBeUndefined();
  });
});

describe('getActiveIntegrationKey', () => {
  it.each([
    [`${Routes.Integrations}${Routes.Model}`, 'model'],
    [`${Routes.Integrations}${Routes.DataSource}`, 'data-source'],
    [
      `${Routes.Integrations}${Routes.DataSource}${Routes.DataSourceDetailPage}`,
      'data-source',
    ],
    [`${Routes.Integrations}${Routes.ChatChannel}`, 'chat-channel'],
    [`${Routes.Integrations}${Routes.Mcp}`, 'mcp'],
  ])('maps %s to %s', (pathname, expected) => {
    expect(getActiveIntegrationKey(pathname)).toBe(expected);
  });

  it('does not match non-integration routes', () => {
    expect(getActiveIntegrationKey('/team')).toBeUndefined();
    expect(getActiveIntegrationKey('/user-setting/profile')).toBeUndefined();
  });
});

describe('getActivePageTab', () => {
  it('returns the context group with the active tab', () => {
    expect(getActivePageTab('/datasets')).toEqual({
      group: expect.objectContaining({ key: NavItemKey.Context }),
      activePath: '/datasets',
    });
    expect(getActivePageTab('/memories')?.activePath).toBe('/memories');
  });

  it('returns the application group with the active tab', () => {
    expect(getActivePageTab('/chats')?.activePath).toBe('/chats');
    expect(getActivePageTab('/searches')?.activePath).toBe('/searches');
  });

  it('returns no group for pages outside tab groups', () => {
    expect(getActivePageTab('/agents')).toBeUndefined();
    expect(getActivePageTab('/dataset/files/1')).toBeUndefined();
    expect(getActivePageTab('/chat/1')).toBeUndefined();
    expect(getActivePageTab('/')).toBeUndefined();
  });
});
