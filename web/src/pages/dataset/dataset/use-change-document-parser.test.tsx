import { renderHook, act } from '@testing-library/react';
import React from 'react';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const mockInvalidate = jest.fn();
const mockSet = jest.fn().mockResolvedValue(0);
const mockRun = jest.fn().mockResolvedValue(undefined);

jest.mock('@tanstack/react-query', () => ({
  useQueryClient: () => ({ invalidateQueries: mockInvalidate }),
}));

jest.mock('@/hooks/use-document-request', () => ({
  useSetDocumentPipelineParser: () => ({
    setDocumentPipelineParser: mockSet,
    loading: false,
  }),
  useRunDocument: () => ({ runDocumentByIds: mockRun }),
  DocumentKeys: { all: () => ['fetchDocumentList'] },
}));

import { DocumentKeys } from '@/hooks/use-document-request';
import { useChangeDocumentParser } from './use-change-document-parser';

const record = {
  id: 'doc-1',
  dataset_id: 'dataset-1',
  name: 'doc.pdf',
  chunk_count: 0,
  pipeline_id: 'canvas-1',
  pipeline_version_id: '',
} as any;

const saveBody = {
  parser_id: '',
  pipeline_id: 'canvas-1',
  pipeline_version_id: 'v1',
  parser_config: {},
  parse_type: 'pipeline',
} as any;

describe('useChangeDocumentParser invalidates the document list after save (R2)', () => {
  it('refetches the document list so the reopened dialog shows the pinned version', async () => {
    const { result } = renderHook(() => useChangeDocumentParser());
    act(() => {
      result.current.showChangeParserModal(record);
    });
    await act(async () => {
      await result.current.onChangeParserOk(saveBody);
    });
    expect(mockInvalidate).toHaveBeenCalledWith({
      queryKey: DocumentKeys.all(),
    });
  });
});
