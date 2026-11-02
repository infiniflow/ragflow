import fileManagerService from '@/services/file-manager-service';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import React from 'react';

import { useFetchFileList } from '../use-file-request';

jest.mock('@/services/file-manager-service', () => ({
  __esModule: true,
  default: { listFile: jest.fn() },
}));

jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

// @/utils/request pulls in the legacy i18n/locales graph that touches
// import.meta.env, which jest cannot transform; the list query never calls it
// directly, so stub the module.
jest.mock('@/utils/request', () => ({ __esModule: true, default: jest.fn() }));

jest.mock('@/utils/file-util', () => ({ downloadFileFromBlob: jest.fn() }));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('react-router', () => ({
  useSearchParams: jest.fn(() => [
    new URLSearchParams('folderId=deleted-folder'),
  ]),
}));

jest.mock('ahooks', () => ({ useDebounce: (value: unknown) => value }));

jest.mock('@/hooks/logic-hooks', () => ({
  useGetPaginationWithRouter: jest.fn(() => ({
    pagination: { current: 1, pageSize: 10 },
    setPagination: jest.fn(),
  })),
  useHandleSearchChange: jest.fn(() => ({
    searchString: '',
    setSearchString: jest.fn(),
    handleInputChange: jest.fn(),
  })),
}));

jest.mock('@/hooks/route-hook', () => ({
  useSetPaginationParams: jest.fn(() => jest.fn()),
}));

const mockListFile = jest.mocked(fileManagerService.listFile);

function makeWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return (props: { children: any }) =>
    React.createElement(
      QueryClientProvider,
      { client: queryClient },
      props.children,
    );
}

describe('useFetchFileList missing folder', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('flags folderNotFound when the folder listing returns 404', async () => {
    mockListFile.mockResolvedValue({ data: { code: 404 } } as never);

    const { result } = renderHook(() => useFetchFileList(), {
      wrapper: makeWrapper(),
    });

    await waitFor(() => expect(result.current.folderNotFound).toBe(true));
    expect(mockListFile).toHaveBeenCalledWith(
      expect.objectContaining({ parent_id: 'deleted-folder' }),
    );
  });

  it('does not flag folderNotFound on a successful listing', async () => {
    mockListFile.mockResolvedValue({
      data: { code: 0, data: { files: [], total: 7 } },
    } as never);

    const { result } = renderHook(() => useFetchFileList(), {
      wrapper: makeWrapper(),
    });

    // initialData.total is 0, so a distinct total proves the successful
    // response was applied before the flag assertion runs.
    await waitFor(() => expect(result.current.total).toBe(7));
    expect(result.current.folderNotFound).toBe(false);
  });
});
