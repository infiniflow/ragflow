import { act, renderHook } from '@testing-library/react';
import { useHandleConnectToKnowledge } from './hooks';
import { useHandleMoveFile } from './use-move-file';

const mockMoveFile = jest.fn(async () => 0);
const mockConnectFileToKnowledge = jest.fn(async () => 0);

jest.mock('@/hooks/use-file-request', () => ({
  useMoveFile: () => ({ moveFile: mockMoveFile, loading: false }),
  useConnectToKnowledge: () => ({
    connectFileToKnowledge: mockConnectFileToKnowledge,
    loading: false,
  }),
  useRenameFile: () => ({ renameFile: jest.fn(), loading: false }),
}));

jest.mock('react-router', () => ({
  useNavigate: () => jest.fn(),
  useSearchParams: () => [new URLSearchParams(), jest.fn()],
}));

describe('file dialogs on folder change', () => {
  beforeEach(() => {
    mockMoveFile.mockClear();
    mockConnectFileToKnowledge.mockClear();
  });

  it('drops the move dialog and its source ids', async () => {
    const { result } = renderHook(() =>
      useHandleMoveFile({ clearRowSelection: jest.fn() }),
    );

    act(() => {
      result.current.showMoveFileModal(['file-a', 'file-b'], true);
    });

    expect(result.current.moveFileVisible).toBe(true);

    act(() => {
      result.current.dismissMoveFile();
    });

    expect(result.current.moveFileVisible).toBe(false);

    await act(async () => {
      await result.current.onMoveFileOk('dest-folder');
    });

    expect(mockMoveFile).toHaveBeenCalledWith({
      src_file_ids: [],
      dest_file_id: 'dest-folder',
    });
  });

  it('drops the link-to-dataset dialog and its file ids', async () => {
    const { result } = renderHook(() => useHandleConnectToKnowledge());

    act(() => {
      result.current.showConnectToKnowledgeModal(['file-a', 'file-b']);
    });

    expect(result.current.connectToKnowledgeVisible).toBe(true);

    act(() => {
      result.current.dismissConnectToKnowledge();
    });

    expect(result.current.connectToKnowledgeVisible).toBe(false);

    await act(async () => {
      await result.current.onConnectToKnowledgeOk([
        { kb_id: 'kb-1', kb_name: 'kb' },
      ]);
    });

    expect(mockConnectFileToKnowledge).toHaveBeenCalledWith(
      expect.objectContaining({ fileIds: [] }),
    );
  });
});
