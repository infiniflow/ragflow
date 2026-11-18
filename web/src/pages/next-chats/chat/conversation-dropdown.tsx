import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  useGetChatSearchParams,
  useRemoveSessions,
} from '@/hooks/use-chat-request';
import { IConversation } from '@/interfaces/database/chat';
import { Trash2 } from 'lucide-react';
import { MouseEventHandler, PropsWithChildren, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useChatStreamStore } from '../chat-stream/store';
import { useChatUrlParams } from '../hooks/use-chat-url';

export function ConversationDropdown({
  children,
  conversation,
  removeTemporaryConversation,
}: PropsWithChildren & {
  conversation: IConversation;
  removeTemporaryConversation?: (conversationId: string) => void;
}) {
  const { t } = useTranslation();
  const { setConversationBoth } = useChatUrlParams();
  const { removeSessions } = useRemoveSessions();
  const { conversationId, isNew } = useGetChatSearchParams();
  const removeStreamSessions = useChatStreamStore(
    (state) => state.removeSessions,
  );

  const handleDelete: MouseEventHandler<HTMLDivElement> =
    useCallback(async () => {
      if (isNew === 'true' && removeTemporaryConversation) {
        removeTemporaryConversation(conversation.id);
        removeStreamSessions([conversation.id]);
        if (conversationId === conversation.id) {
          setConversationBoth('', '');
        }
      } else {
        const code = await removeSessions([conversation.id]);
        if (code === 0) {
          removeStreamSessions([conversation.id]);
          setConversationBoth('', '');
        }
      }
    }, [
      conversation.id,
      conversationId,
      isNew,
      removeSessions,
      removeStreamSessions,
      removeTemporaryConversation,
      setConversationBoth,
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
            data-testid="chat-detail-session-delete"
            data-session-id={conversation.id}
          >
            {t('common.delete')} <Trash2 />
          </DropdownMenuItem>
        </ConfirmDeleteDialog>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
