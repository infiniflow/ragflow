import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { IFile } from '@/interfaces/database/file-manager';
import { CellContext } from '@tanstack/react-table';
import { EllipsisVertical, Link2, Trash2 } from 'lucide-react';
import { useCallback } from 'react';
import { UseHandleConnectToKnowledgeReturnType } from './hooks';

type IProps = Pick<CellContext<IFile, unknown>, 'row'> &
  Pick<UseHandleConnectToKnowledgeReturnType, 'showConnectToKnowledgeModal'>;

export function ActionCell({ row, showConnectToKnowledgeModal }: IProps) {
  const record = row.original;

  const handleShowConnectToKnowledgeModal = useCallback(() => {
    showConnectToKnowledgeModal(record);
  }, [record, showConnectToKnowledgeModal]);

  return (
    <section className="flex gap-4 items-center">
      <Button
        variant="secondary"
        size={'icon'}
        onClick={handleShowConnectToKnowledgeModal}
      >
        <Link2 />
      </Button>
      <ConfirmDeleteDialog>
        <Button variant="secondary" size={'icon'}>
          <Trash2 />
        </Button>
      </ConfirmDeleteDialog>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="secondary" size={'icon'}>
            <EllipsisVertical />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuLabel>Actions</DropdownMenuLabel>
          <DropdownMenuItem
            onClick={() => navigator.clipboard.writeText(record.id)}
          >
            Copy payment ID
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem>View customer</DropdownMenuItem>
          <DropdownMenuItem>View payment details</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </section>
  );
}
