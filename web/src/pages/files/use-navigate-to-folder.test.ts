import { renderHook } from '@testing-library/react';

jest.mock('@/hooks/use-file-request', () => ({
  useGetFolderId: jest.fn(),
  useFetchParentFolderList: jest.fn(),
}));

jest.mock('@/hooks/logic-hooks/navigate-hooks', () => ({
  useNavigatePage: () => ({ navigateToFiles: jest.fn() }),
}));

import { useCurrentFolder } from './use-navigate-to-folder';

const mockUseGetFolderId = jest.requireMock('@/hooks/use-file-request')
  .useGetFolderId as jest.Mock;
const mockUseFetchParentFolderList = jest.requireMock(
  '@/hooks/use-file-request',
).useFetchParentFolderList as jest.Mock;

function createFolder(id: string, createTime: number) {
  return {
    id,
    create_time: createTime,
    create_date: '',
    created_by: 'user-1',
    location: '',
    name: id,
    parent_id: 'root',
    size: 0,
    tenant_id: 'tenant-1',
    type: 'folder',
    update_date: '',
    update_time: createTime,
    source_type: '',
  };
}

const rootFolder = { ...createFolder('root', 100), parent_id: 'root' };
const firstFolder = createFolder('folder-1', 200);
const secondFolder = createFolder('folder-2', 300);

describe('useCurrentFolder', () => {
  beforeEach(() => {
    mockUseGetFolderId.mockReset();
    mockUseFetchParentFolderList.mockReset();
    mockUseFetchParentFolderList.mockReturnValue([]);
  });

  it('uses the list response root folder at the library root', () => {
    mockUseGetFolderId.mockReturnValue('');

    const { result } = renderHook(() => useCurrentFolder(rootFolder));

    expect(result.current).toBe(rootFolder);
  });

  it('uses the current ancestor instead of the parent folder for a subfolder', () => {
    mockUseGetFolderId.mockReturnValue(firstFolder.id);
    mockUseFetchParentFolderList.mockReturnValue([rootFolder, firstFolder]);

    const { result } = renderHook(() => useCurrentFolder(rootFolder));

    expect(result.current).toBe(firstFolder);
  });

  it('does not show the parent time while the current folder path is loading', () => {
    mockUseGetFolderId.mockReturnValue(firstFolder.id);

    const { result } = renderHook(() => useCurrentFolder(rootFolder));

    expect(result.current).toBeUndefined();
  });

  it('rejects a stale folder path that belongs to another folder', () => {
    mockUseGetFolderId.mockReturnValue(secondFolder.id);
    mockUseFetchParentFolderList.mockReturnValue([rootFolder, firstFolder]);

    const { result } = renderHook(() => useCurrentFolder(rootFolder));

    expect(result.current).toBeUndefined();
  });

  it('follows the current folder when navigation changes', () => {
    mockUseGetFolderId.mockReturnValue(firstFolder.id);
    mockUseFetchParentFolderList.mockReturnValue([rootFolder, firstFolder]);
    const { result, rerender } = renderHook(() => useCurrentFolder(rootFolder));

    expect(result.current).toBe(firstFolder);

    mockUseGetFolderId.mockReturnValue(secondFolder.id);
    mockUseFetchParentFolderList.mockReturnValue([
      rootFolder,
      firstFolder,
      secondFolder,
    ]);
    rerender();

    expect(result.current).toBe(secondFolder);
  });
});
