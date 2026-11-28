import { IFile } from '@/interfaces/database/file-manager';
import { OnChangeFn, RowSelectionState } from '@tanstack/react-table';
import { FolderInput, Link2, Trash2 } from 'lucide-react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { UseHandleConnectToKnowledgeReturnType } from './hooks';
import { useHandleDeleteFile } from './use-delete-file';
import { UseMoveDocumentShowType } from './use-move-file';

export function useBulkOperateFile({
  files,
  rowSelection,
  showMoveFileModal,
  setRowSelection,
  showConnectToKnowledgeModal,
}: {
  files: IFile[];
  rowSelection: RowSelectionState;
  setRowSelection: OnChangeFn<RowSelectionState>;
} & UseMoveDocumentShowType & {
    showConnectToKnowledgeModal: UseHandleConnectToKnowledgeReturnType['showConnectToKnowledgeModal'];
  }) {
  const { t } = useTranslation();

  // A row can be checked while the debounced search still shows the old list.
  const selectedIds = useMemo(
    () =>
      (files ?? []).filter(({ id }) => rowSelection[id]).map(({ id }) => id),
    [files, rowSelection],
  );

  const { handleRemoveFile } = useHandleDeleteFile();

  const list = [
    {
      id: 'move',
      label: t('common.move'),
      icon: <FolderInput />,
      onClick: () => {
        showMoveFileModal(selectedIds, true);
      },
    },
    {
      id: 'link',
      label: t('fileManager.addToKnowledge'),
      icon: <Link2 />,
      onClick: async () => {
        showConnectToKnowledgeModal(selectedIds);
      },
    },
    {
      id: 'delete',
      label: t('common.delete'),
      icon: <Trash2 />,
      onClick: async () => {
        const code = await handleRemoveFile(selectedIds);
        if (code === 0) {
          setRowSelection({});
        }
      },
    },
  ];

  return { list, selectedCount: selectedIds.length };
}
