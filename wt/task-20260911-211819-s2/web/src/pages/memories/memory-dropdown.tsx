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
import { PenLine, Trash2 } from 'lucide-react';
import { MouseEventHandler, PropsWithChildren, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useDeleteMemory } from './hooks';
import { IMemory } from './interface';

export function MemoryDropdown({
  children,
  memory,
  showMemoryRenameModal,
}: PropsWithChildren & {
  memory: IMemory;
  showMemoryRenameModal: (memory: IMemory) => void;
}) {
  const { t } = useTranslation();
  const { deleteMemory } = useDeleteMemory();
  const handleShowChatRenameModal: MouseEventHandler<HTMLDivElement> =
    useCallback(
      (e) => {
        e.stopPropagation();
        showMemoryRenameModal(memory);
      },
      [memory, showMemoryRenameModal],
    );
  const handleDelete: MouseEventHandler<HTMLDivElement> = useCallback(() => {
    deleteMemory({ memory_id: memory.id });
  }, [memory, deleteMemory]);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>{children}</DropdownMenuTrigger>
      <DropdownMenuContent>
        <DropdownMenuItem onClick={handleShowChatRenameModal}>
          {t('common.rename')} <PenLine />
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <ConfirmDeleteDialog
          onOk={handleDelete}
          title={t('deleteModal.delMemory')}
          content={{
            node: (
              <ConfirmDeleteDialogNode
                avatar={{ avatar: memory.avatar, name: memory.name }}
                name={memory.name}
                warnText={t('memories.delMemoryWarn')}
              />
            ),
          }}
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
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
