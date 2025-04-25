import { IFile } from '@/interfaces/database/file-manager';
import { OnChangeFn, RowSelectionState } from '@tanstack/react-table';
import { FolderInput, Trash2 } from 'lucide-react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useHandleDeleteFile } from './use-delete-file';
import { UseMoveDocumentShowType } from './use-move-file';

export function useBulkOperateFile({
  files,
  rowSelection,
  showMoveFileModal,
  setRowSelection,
}: {
  files: IFile[];
  rowSelection: RowSelectionState;
  setRowSelection: OnChangeFn<RowSelectionState>;
} & UseMoveDocumentShowType) {
  const { t } = useTranslation();

  const selectedIds = useMemo(() => {
    const indexes = Object.keys(rowSelection);
    return files
      .filter((x, idx) => indexes.some((y) => Number(y) === idx))
      .map((x) => x.id);
  }, [files, rowSelection]);

  const { handleRemoveFile } = useHandleDeleteFile();

  const list = [
    {
      id: 'move',
      label: t('common.move'),
      icon: <FolderInput />,
      onClick: () => {
        showMoveFileModal(selectedIds);
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

  return { list };
}
