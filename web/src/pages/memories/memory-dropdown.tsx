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
import {
  MemoryDropdownGuard,
  shouldShowMemoryDropdown,
} from '@/ee/pages/memories/memory-dropdown-guard';
import { PenLine, Trash2 } from 'lucide-react';
import { MouseEventHandler, PropsWithChildren, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useDeleteMemory } from './hooks';
import { IMemory } from './interface';

export function MemoryDropdown({
  children,
  memory,
  showMemoryRenameModal,
  showPrivilegeModal,
}: PropsWithChildren & {
  memory: IMemory;
  showMemoryRenameModal: (memory: IMemory) => void;
  showPrivilegeModal(): void;
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

  if (!shouldShowMemoryDropdown(memory)) {
    return null;
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>{children}</DropdownMenuTrigger>
      <DropdownMenuContent>
        <MemoryDropdownGuard
          memory={memory}
          showPrivilegeModal={showPrivilegeModal}
          manageItems={
            <DropdownMenuItem
              justifyBetween={false}
              onClick={handleShowChatRenameModal}
            >
              <PenLine /> {t('common.rename')}
            </DropdownMenuItem>
          }
          deleteItems={
            <>
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
                  justifyBetween={false}
                  className="text-state-error"
                  onSelect={(e) => {
                    e.preventDefault();
                  }}
                  onClick={(e) => {
                    e.stopPropagation();
                  }}
                >
                  <Trash2 /> {t('common.delete')}
                </DropdownMenuItem>
              </ConfirmDeleteDialog>
            </>
          }
        />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
