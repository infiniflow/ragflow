import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useRemoveConversation } from '@/hooks/use-chat-request';
import { IConversation } from '@/interfaces/database/chat';
import { Trash2 } from 'lucide-react';
import { MouseEventHandler, PropsWithChildren, useCallback } from 'react';
import { useTranslation } from 'react-i18next';

export function ConversationDropdown({
  children,
  conversation,
  removeTemporaryConversation,
}: PropsWithChildren & {
  conversation: IConversation;
  removeTemporaryConversation?: (conversationId: string) => void;
}) {
  const { t } = useTranslation();

  const { removeConversation } = useRemoveConversation();

  const handleDelete: MouseEventHandler<HTMLDivElement> = useCallback(() => {
    if (conversation.is_new && removeTemporaryConversation) {
      removeTemporaryConversation(conversation.id);
      removeConversation([]);
    } else {
      removeConversation([conversation.id]);
    }
  }, [
    conversation.id,
    conversation.is_new,
    removeConversation,
    removeTemporaryConversation,
  ]);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>{children}</DropdownMenuTrigger>
      <DropdownMenuContent>
        <ConfirmDeleteDialog onOk={handleDelete}>
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
