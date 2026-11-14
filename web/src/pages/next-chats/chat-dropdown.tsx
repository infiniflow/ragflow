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
import { ChatDropdownGuard } from '@/ee/pages/next-chats/chat-dropdown-guard';
import { useDeleteChat } from '@/hooks/use-chat-request';
import { IDialog } from '@/interfaces/database/chat';
import { PenLine, Trash2 } from 'lucide-react';
import { MouseEventHandler, PropsWithChildren, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useRenameChat } from './hooks/use-rename-chat';

export function ChatDropdown({
  chat,
  children,
  showChatRenameModal,
  showPrivilegeModal,
}: PropsWithChildren &
  Pick<ReturnType<typeof useRenameChat>, 'showChatRenameModal'> & {
    chat: IDialog;
    showPrivilegeModal(): void;
  }) {
  const { t } = useTranslation();
  const { deleteChat } = useDeleteChat();

  const handleShowChatRenameModal: MouseEventHandler<HTMLDivElement> =
    useCallback(
      (e) => {
        e.stopPropagation();
        showChatRenameModal(chat);
      },
      [chat, showChatRenameModal],
    );

  const handleDelete: MouseEventHandler<HTMLDivElement> = useCallback(() => {
    deleteChat(chat.id);
  }, [chat.id, deleteChat]);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>{children}</DropdownMenuTrigger>
      <DropdownMenuContent>
        <ChatDropdownGuard
          chat={chat}
          showPrivilegeModal={showPrivilegeModal}
          manageItems={
            <DropdownMenuItem onClick={handleShowChatRenameModal}>
              {t('common.rename')} <PenLine />
            </DropdownMenuItem>
          }
          deleteItems={
            <>
              <DropdownMenuSeparator />
              <ConfirmDeleteDialog
                onOk={handleDelete}
                title={t('deleteModal.delChat')}
                content={{
                  node: (
                    <ConfirmDeleteDialogNode
                      avatar={{ avatar: chat.icon, name: chat.name }}
                      name={chat.name}
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
            </>
          }
        />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
