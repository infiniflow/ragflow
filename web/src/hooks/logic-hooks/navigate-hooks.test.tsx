import { Routes as AppRoutes } from '@/routes';
import { act, renderHook } from '@testing-library/react';
import { PropsWithChildren } from 'react';
import { MemoryRouter, useLocation } from 'react-router';
import { useNavigatePage } from './navigate-hooks';

function createWrapper(initialPath: string) {
  return function Wrapper({ children }: PropsWithChildren) {
    return (
      <MemoryRouter initialEntries={[initialPath]}>{children}</MemoryRouter>
    );
  };
}

function renderNavigatePage(initialPath: string) {
  return renderHook(
    () => ({
      navigation: useNavigatePage(),
      location: useLocation(),
    }),
    { wrapper: createWrapper(initialPath) },
  );
}

describe('useNavigatePage', () => {
  it('returns to the original location after switching between settings pages', () => {
    const { result } = renderNavigatePage('/datasets?isCreate=true#list');

    act(() => {
      result.current.navigation.navigateToUserSettingProfile();
    });

    expect(result.current.location.pathname).toBe('/user-setting/profile');
    expect(result.current.location.state).toEqual({
      returnTo: '/datasets?isCreate=true#list',
    });

    act(() => {
      result.current.navigation.navigateToUserSettingApi();
    });

    expect(result.current.location.pathname).toBe('/user-setting/api');
    expect(result.current.location.state).toEqual({
      returnTo: '/datasets?isCreate=true#list',
    });

    act(() => {
      result.current.navigation.closeUserSetting();
    });

    expect(result.current.location.pathname).toBe('/datasets');
    expect(result.current.location.search).toBe('?isCreate=true');
    expect(result.current.location.hash).toBe('#list');
  });

  it('builds the settings path from a menu section', () => {
    const { result } = renderNavigatePage('/datasets');

    act(() => {
      result.current.navigation.navigateToUserSetting('/team');
    });

    expect(result.current.location.pathname).toBe('/user-setting/team');
    expect(result.current.location.state).toEqual({ returnTo: '/datasets' });
  });

  it('falls back to datasets when settings are opened directly', () => {
    const { result } = renderNavigatePage('/user-setting/profile');

    act(() => {
      result.current.navigation.closeUserSetting();
    });

    expect(result.current.location.pathname).toBe('/datasets');
  });

  it('navigates to agent explore with session parameters', () => {
    const { result } = renderNavigatePage('/agents');

    act(() => {
      result.current.navigation.navigateToAgentExplore('canvas-1', {
        sessionId: 'session 1',
        isNew: true,
      });
    });

    expect(result.current.location.pathname).toBe('/agent/canvas-1/explore');
    expect(result.current.location.search).toBe(
      '?sessionId=session+1&isNew=true',
    );

    act(() => {
      result.current.navigation.navigateToAgentExplore('canvas-2');
    });

    expect(result.current.location.pathname).toBe('/agent/canvas-2/explore');
    expect(result.current.location.search).toBe('');
  });

  it('navigates to dataset pages and sections with state', () => {
    const { result } = renderNavigatePage('/datasets');

    act(() => {
      result.current.navigation.navigateToDataset('dataset-1')();
    });

    expect(result.current.location.pathname).toBe('/dataset/files/dataset-1');

    act(() => {
      result.current.navigation.navigateToDatasetSection(
        'dataset-1',
        AppRoutes.DataSetSetting,
        { from: 'logs' },
      )();
    });

    expect(result.current.location.pathname).toBe(
      '/dataset/configuration/dataset-1',
    );
    expect(result.current.location.state).toEqual({ from: 'logs' });
  });

  it('navigates to memory sections', () => {
    const { result } = renderNavigatePage('/memories');

    act(() => {
      result.current.navigation.navigateToMemorySection(
        'memory-1',
        AppRoutes.MemorySetting,
      )();
    });

    expect(result.current.location.pathname).toBe(
      '/memory/memory-setting/memory-1',
    );
  });

  it('navigates to file roots, folders, and skills', () => {
    const { result } = renderNavigatePage('/datasets');

    act(() => {
      result.current.navigation.navigateToFolder()();
    });

    expect(result.current.location.pathname).toBe('/files');
    expect(result.current.location.search).toBe('');

    act(() => {
      result.current.navigation.navigateToFolder('folder & 1')();
    });

    expect(result.current.location.search).toBe('?folderId=folder+%26+1');

    act(() => {
      result.current.navigation.navigateToSkills();
    });

    expect(result.current.location.pathname).toBe('/files/skills');
  });

  it('navigates to application lists with the creation parameter', () => {
    const { result } = renderNavigatePage('/');

    act(() => {
      result.current.navigation.navigateToApplicationList(AppRoutes.Chats, {
        isCreate: true,
      })();
    });

    expect(result.current.location.pathname).toBe('/chats');
    expect(result.current.location.search).toBe('?isCreate=true');

    act(() => {
      result.current.navigation.navigateToApplicationList(AppRoutes.Searches)();
    });

    expect(result.current.location.pathname).toBe('/searches');
    expect(result.current.location.search).toBe('');
  });

  it('restores the compilation tab only when returning to agents', () => {
    const { result } = renderNavigatePage(
      '/compilation-templates/edit-next/group-1?source=agents',
    );

    act(() => {
      result.current.navigation.navigateToAgentsFromCompilation('agents')();
    });

    expect(result.current.location.pathname).toBe('/agents');
    expect(result.current.location.search).toBe(
      '?canvas_category=compilation_template_group',
    );

    act(() => {
      result.current.navigation.navigateToAgentsFromCompilation('settings')();
    });

    expect(result.current.location.pathname).toBe('/agents');
    expect(result.current.location.search).toBe('');
  });

  it('navigates from a missing page to the matching home page', () => {
    const { result } = renderNavigatePage('/admin/missing');

    act(() => {
      result.current.navigation.navigateToDefaultPage('/admin/missing')();
    });

    expect(result.current.location.pathname).toBe('/admin');

    act(() => {
      result.current.navigation.navigateToDefaultPage('/missing')();
    });

    expect(result.current.location.pathname).toBe('/');
  });
});
