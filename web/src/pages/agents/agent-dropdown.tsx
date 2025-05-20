import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useDeleteKnowledge } from '@/hooks/use-knowledge-request';
import { IFlow } from '@/interfaces/database/flow';
import { PenLine, Trash2 } from 'lucide-react';
import { MouseEventHandler, PropsWithChildren, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useRenameAgent } from './use-rename-agent';

export function AgentDropdown({
  children,
  showDatasetRenameModal,
  dataset,
}: PropsWithChildren &
  Pick<ReturnType<typeof useRenameAgent>, 'showDatasetRenameModal'> & {
    dataset: IFlow;
  }) {
  const { t } = useTranslation();
  const { deleteKnowledge } = useDeleteKnowledge();

  const handleShowDatasetRenameModal: MouseEventHandler<HTMLDivElement> =
    useCallback(
      (e) => {
        e.stopPropagation();
        showDatasetRenameModal(dataset);
      },
      [dataset, showDatasetRenameModal],
    );

  const handleDelete: MouseEventHandler<HTMLDivElement> = useCallback(() => {
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
            onSelect={(e) => {
              e.preventDefault();
            }}
            onClick={(e) => {
              e.stopPropagation();
            }}
          >
            {t('common.delete')} <Trash2 />
          </DropdownMenuItem>
        </ConfirmDeleteDialog>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
