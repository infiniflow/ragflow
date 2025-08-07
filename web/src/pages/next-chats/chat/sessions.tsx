import { MoreButton } from '@/components/more-button';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { useGetChatSearchParams } from '@/hooks/use-chat-request';
import { cn } from '@/lib/utils';
import { Plus } from 'lucide-react';
import { useCallback } from 'react';
import { useHandleClickConversationCard } from '../hooks/use-click-card';
import { useSelectDerivedConversationList } from '../hooks/use-select-conversation-list';
import { ChatSettingSheet } from './app-settings/chat-settings-sheet';

type SessionProps = Pick<
  ReturnType<typeof useHandleClickConversationCard>,
  'handleConversationCardClick'
>;
export function Sessions({ handleConversationCardClick }: SessionProps) {
  const { list: conversationList, addTemporaryConversation } =
    useSelectDerivedConversationList();

  const handleCardClick = useCallback(
    (conversationId: string, isNew: boolean) => () => {
      handleConversationCardClick(conversationId, isNew);
    },
    [handleConversationCardClick],
  );

  const { conversationId } = useGetChatSearchParams();

  return (
    <section className="p-6 w-[400px] max-w-[20%] flex flex-col">
      <div className="flex justify-between items-center mb-4">
        <span className="text-xl font-bold">Conversations</span>
        <Button variant={'ghost'} onClick={addTemporaryConversation}>
          <Plus></Plus>
        </Button>
      </div>
      <div className="space-y-4 flex-1 overflow-auto">
        {conversationList.map((x) => (
          <Card
            key={x.id}
            onClick={handleCardClick(x.id, x.is_new)}
            className={cn('cursor-pointer bg-transparent', {
              'bg-bg-card': conversationId === x.id,
            })}
          >
            <CardContent className="px-3 py-2 flex justify-between items-center group">
              {x.name}
              <MoreButton></MoreButton>
            </CardContent>
          </Card>
        ))}
      </div>
      <div className="py-2">
        <ChatSettingSheet>
          <Button className="w-full">Chat Settings</Button>
        </ChatSettingSheet>
      </div>
    </section>
  );
}
