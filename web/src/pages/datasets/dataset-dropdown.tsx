import {
  ConfirmDeleteDialog,
  ConfirmDeleteDialogNode,
} from '@/components/confirm-delete-dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { DatasetDropdownGuard } from '@/ee/pages/datasets/dataset-dropdown-guard';
import useDuplicateDataset from '@/ee/pages/datasets/use-duplicate-dataset';
import { useDeleteKnowledge } from '@/hooks/use-knowledge-request';
import { IDataset } from '@/interfaces/database/dataset';
import { PenLine, Trash2 } from 'lucide-react';
import { MouseEventHandler, PropsWithChildren, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useRenameDataset } from './use-rename-dataset';

type IDatasetDropdownProps = {
  dataset: IDataset;
  showPrivilegeModal(): void;
} & Pick<ReturnType<typeof useRenameDataset>, 'showDatasetRenameModal'> & {
    showDatasetDuplicateModal: ReturnType<
      typeof useDuplicateDataset
    >['showModal'];
  };

export function DatasetDropdown({
  children,
  showDatasetRenameModal,
  dataset,
  showPrivilegeModal,
}: PropsWithChildren<IDatasetDropdownProps> &
  Pick<ReturnType<typeof useRenameDataset>, 'showDatasetRenameModal'> & {
    dataset: IDataset;
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
        <DatasetDropdownGuard
          dataset={dataset}
          showPrivilegeModal={showPrivilegeModal}
          manageItems={
            <DropdownMenuItem onClick={handleShowDatasetRenameModal}>
              {t('common.rename')} <PenLine />
            </DropdownMenuItem>
          }
          deleteItems={
            <>
              <DropdownMenuSeparator />
              <ConfirmDeleteDialog
                onOk={handleDelete}
                content={{
                  node: (
                    <ConfirmDeleteDialogNode
                      avatar={{ avatar: dataset.avatar, name: dataset.name }}
                      name={dataset.name}
                    />
                  ),
                }}
                title={t('deleteModal.delDataset')}
              >
                <DropdownMenuItem
                  className="text-state-error"
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
            </>
          }
        />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
