import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useDeleteKnowledge } from '@/hooks/use-knowledge-request';
import { IKnowledge } from '@/interfaces/database/knowledge';
import { PenLine, Trash2 } from 'lucide-react';
import { PropsWithChildren, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useRenameDataset } from './use-rename-dataset';

export function DatasetDropdown({
  children,
  showDatasetRenameModal,
  dataset,
}: PropsWithChildren &
  Pick<ReturnType<typeof useRenameDataset>, 'showDatasetRenameModal'> & {
    dataset: IKnowledge;
  }) {
  const { t } = useTranslation();
  const { deleteKnowledge } = useDeleteKnowledge();

  const handleShowDatasetRenameModal = useCallback(() => {
    showDatasetRenameModal(dataset);
  }, [dataset, showDatasetRenameModal]);

  const handleDelete = useCallback(() => {
    deleteKnowledge(dataset.id);
  }, [dataset.id, deleteKnowledge]);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>{children}</DropdownMenuTrigger>
      <DropdownMenuContent>
        <DropdownMenuItem onClick={handleShowDatasetRenameModal}>
          {t('common.rename')} <PenLine />
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <ConfirmDeleteDialog onOk={handleDelete}>
          <DropdownMenuItem
            className="text-text-delete-red"
            onSelect={(e) => e.preventDefault()}
          >
            {t('common.delete')} <Trash2 />
          </DropdownMenuItem>
        </ConfirmDeleteDialog>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
