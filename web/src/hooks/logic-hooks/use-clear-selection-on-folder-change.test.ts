import { renderHook } from '@testing-library/react';
import { useClearSelectionOnFolderChange } from './use-clear-selection-on-folder-change';

describe('useClearSelectionOnFolderChange', () => {
  it('does not clear on the initial render', () => {
    const clearSelection = jest.fn();

    renderHook(() =>
      useClearSelectionOnFolderChange('folder-a', clearSelection),
    );

    expect(clearSelection).not.toHaveBeenCalled();
  });

  it('clears once when the folder id changes', () => {
    const clearSelection = jest.fn();
    const { rerender } = renderHook(
      ({ folderId }) =>
        useClearSelectionOnFolderChange(folderId, clearSelection),
      { initialProps: { folderId: 'folder-a' } },
    );

    rerender({ folderId: 'folder-b' });

    expect(clearSelection).toHaveBeenCalledTimes(1);
  });

  it('does not clear again for the same folder id or a new clear callback', () => {
    const clearSelection = jest.fn();
    const nextClearSelection = jest.fn();
    const { rerender } = renderHook(
      ({
        folderId,
        clearSelection: clear,
      }: {
        folderId: string;
        clearSelection: () => void;
      }) => useClearSelectionOnFolderChange(folderId, clear),
      {
        initialProps: { folderId: 'folder-a', clearSelection },
      },
    );

    rerender({ folderId: 'folder-b', clearSelection });
    rerender({ folderId: 'folder-b', clearSelection: nextClearSelection });

    expect(clearSelection).toHaveBeenCalledTimes(1);
    expect(nextClearSelection).not.toHaveBeenCalled();
  });

  it('clears when navigating from a folder back to the root', () => {
    const clearSelection = jest.fn();
    const { rerender } = renderHook(
      ({ folderId }) =>
        useClearSelectionOnFolderChange(folderId, clearSelection),
      { initialProps: { folderId: 'folder-a' } },
    );

    rerender({ folderId: '' });

    expect(clearSelection).toHaveBeenCalledTimes(1);
  });
});
