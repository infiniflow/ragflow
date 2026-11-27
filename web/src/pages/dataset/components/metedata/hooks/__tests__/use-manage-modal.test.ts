import { DocumentKeys } from '@/hooks/document-query-keys';
import {
  getMetaDataService,
  updateDocumentsMetadata,
} from '@/services/knowledge-service';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import React from 'react';
import { MetadataType } from '../../constant';
import { useManageMetaDataModal } from '../use-manage-modal';

jest.mock('@/services/knowledge-service', () => ({
  getMetaDataService: jest.fn(),
  kbUpdateMetaData: jest.fn(),
  updateDocumentMetaDataConfig: jest.fn(),
  updateDocumentsMetadata: jest.fn(),
}));

jest.mock(
  '@/ee/pages/dataset/components/metedata/hooks/use-save-builtin-metadata',
  () => ({
    useSaveBuiltinMetadata: () => ({ saveBuiltinMetadata: jest.fn() }),
  }),
);

jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { success: jest.fn(), warning: jest.fn(), error: jest.fn() },
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('react-router', () => ({
  useParams: () => ({ id: 'dataset-id' }),
}));

const mockGetMetaData = jest.mocked(getMetaDataService);
const mockUpdateDocumentsMetadata = jest.mocked(updateDocumentsMetadata);

describe('useManageMetaDataModal', () => {
  it('marks the file list filter counts stale after saving metadata', async () => {
    mockGetMetaData.mockResolvedValue({
      data: { data: { summary: {} } },
    } as never);
    mockUpdateDocumentsMetadata.mockResolvedValue({
      data: { code: 0 },
    } as never);

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const filterKey = DocumentKeys.filter('dataset-id');
    queryClient.setQueryData(filterKey, { filter: { metadata: {} } });
    // esbuild-jest loads this .ts file without JSX, so the provider element is
    // built with createElement.
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const wrapper = (props: { children: any }) =>
      React.createElement(
        QueryClientProvider,
        { client: queryClient },
        props.children,
      );

    const { result } = renderHook(
      () =>
        useManageMetaDataModal([], MetadataType.Manage, undefined, ['doc-1']),
      { wrapper },
    );

    const callback = jest.fn();
    await act(async () => {
      await result.current.handleSave({ callback });
    });

    await waitFor(() => expect(callback).toHaveBeenCalledTimes(1));
    expect(mockUpdateDocumentsMetadata).toHaveBeenCalledTimes(1);
    expect(queryClient.getQueryState(filterKey)?.isInvalidated).toBe(true);
  });
});
