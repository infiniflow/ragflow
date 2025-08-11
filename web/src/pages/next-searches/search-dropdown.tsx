import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Trash2 } from 'lucide-react';
import { MouseEventHandler, PropsWithChildren, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { ISearchAppProps, useDeleteSearch } from './hooks';

export function SearchDropdown({
  children,
  dataset,
}: PropsWithChildren & {
  dataset: ISearchAppProps;
}) {
  const { t } = useTranslation();
  const { deleteSearch } = useDeleteSearch();

  const handleDelete: MouseEventHandler<HTMLDivElement> = useCallback(() => {
    deleteSearch({ search_id: dataset.id });
  }, [dataset.id, deleteSearch]);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>{children}</DropdownMenuTrigger>
      <DropdownMenuContent>
        {/* <DropdownMenuItem onClick={handleShowDatasetRenameModal}>
          {t('common.rename')} <PenLine />
        </DropdownMenuItem>
        <DropdownMenuSeparator /> */}
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
