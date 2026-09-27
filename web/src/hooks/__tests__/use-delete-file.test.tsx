import message from '@/components/ui/message';
import fileManagerService from '@/services/file-manager-service';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook } from '@testing-library/react';
import React from 'react';

import { useDeleteFile } from '../use-file-request';

jest.mock('@/services/file-manager-service', () => ({
  __esModule: true,
  default: { removeFile: jest.fn() },
}));

jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

// @/utils/request pulls in the legacy i18n/locales graph that touches
// import.meta.env, which jest cannot transform; the delete mutation never
// calls it directly, so stub the module.
jest.mock('@/utils/request', () => ({ __esModule: true, default: jest.fn() }));

jest.mock('@/utils/file-util', () => ({ downloadFileFromBlob: jest.fn() }));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('react-router', () => ({
  useSearchParams: jest.fn(() => [new URLSearchParams()]),
}));

jest.mock('ahooks', () => ({ useDebounce: (value: unknown) => value }));

// Sibling hooks imported by use-file-request pull in the legacy locales graph
// via logic-hooks; the delete mutation never calls them, so stub both.
jest.mock('@/hooks/logic-hooks', () => ({
  useGetPaginationWithRouter: jest.fn(() => ({})),
  useHandleSearchChange: jest.fn(() => ({})),
}));

jest.mock('@/hooks/route-hook', () => ({
  useSetPaginationParams: jest.fn(() => jest.fn()),
}));

const mockRemoveFile = jest.mocked(fileManagerService.removeFile);
const mockMessage = jest.mocked(message);

function makeWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  // esbuild-jest config here loads .ts with the "tsx" loader but .tsx with the
  // plain "ts" loader, so JSX in this test file would not transform. Build the
  // provider element with createElement instead.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const Wrapper = (props: { children: any }) =>
    React.createElement(
      QueryClientProvider,
      { client: queryClient },
      props.children,
    );
  return Wrapper;
}

describe('useDeleteFile', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('returns the backend code and toasts on success', async () => {
    mockRemoveFile.mockResolvedValue({ data: { code: 0 } } as never);

    const { result } = renderHook(() => useDeleteFile(), {
      wrapper: makeWrapper(),
    });

    await expect(
      result.current.deleteFile({ fileIds: ['f1'], parentId: 'root' }),
    ).resolves.toBe(0);
    expect(mockRemoveFile).toHaveBeenCalledWith({ ids: ['f1'] });
    expect(mockMessage.success).toHaveBeenCalled();
  });

  it('passes through a non-zero code without a success toast', async () => {
    mockRemoveFile.mockResolvedValue({ data: { code: 100 } } as never);

    const { result } = renderHook(() => useDeleteFile(), {
      wrapper: makeWrapper(),
    });

    await expect(
      result.current.deleteFile({ fileIds: ['f1'], parentId: 'root' }),
    ).resolves.toBe(100);
    expect(mockMessage.success).not.toHaveBeenCalled();
  });

  it('resolves to undefined instead of rejecting when the request fails', async () => {
    mockRemoveFile.mockRejectedValue(new Error('network down'));

    const { result } = renderHook(() => useDeleteFile(), {
      wrapper: makeWrapper(),
    });

    // Callers (single-row and bulk delete) check `code === 0`, so an
    // undefined result leaves the row selection untouched instead of
    // surfacing an unhandled rejection.
    await expect(
      result.current.deleteFile({ fileIds: ['f1'], parentId: 'root' }),
    ).resolves.toBeUndefined();
    expect(mockMessage.success).not.toHaveBeenCalled();
  });
});
