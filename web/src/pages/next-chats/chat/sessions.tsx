import { MoreButton } from '@/components/more-button';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { useSetModalState } from '@/hooks/common-hooks';
import {
  useFetchDialog,
  useGetChatSearchParams,
} from '@/hooks/use-chat-request';
import { cn } from '@/lib/utils';
import { PanelLeftClose, PanelRightClose, Plus } from 'lucide-react';
import { useCallback } from 'react';
import { useHandleClickConversationCard } from '../hooks/use-click-card';
import { useSelectDerivedConversationList } from '../hooks/use-select-conversation-list';

type SessionProps = Pick<
  ReturnType<typeof useHandleClickConversationCard>,
  'handleConversationCardClick'
> & { switchSettingVisible(): void };
export function Sessions({
  handleConversationCardClick,
  switchSettingVisible,
}: SessionProps) {
  const { list: conversationList, addTemporaryConversation } =
    useSelectDerivedConversationList();
  const { data } = useFetchDialog();
  const { visible, switchVisible } = useSetModalState(true);

  const handleCardClick = useCallback(
    (conversationId: string, isNew: boolean) => () => {
      handleConversationCardClick(conversationId, isNew);
    },
    [handleConversationCardClick],
  );

  const { conversationId } = useGetChatSearchParams();

  if (!visible) {
    return (
      <PanelRightClose
        className="cursor-pointer size-4 mt-8"
        onClick={switchVisible}
      />
    );
  }

  return (
    <section className="p-6 w-[400px] max-w-[20%] flex flex-col">
      <section className="flex items-center text-base justify-between gap-2">
        <div className="flex gap-3 items-center min-w-0">
          <RAGFlowAvatar
            avatar={data.icon}
            name={data.name}
            className="size-8"
          ></RAGFlowAvatar>
          <span className="flex-1 truncate">{data.name}</span>
        </div>
        <PanelLeftClose
          className="cursor-pointer size-4"
          onClick={switchVisible}
        />
      </section>
      <div className="flex justify-between items-center mb-4 pt-10">
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
        <Button className="w-full" onClick={switchSettingVisible}>
          Chat Settings
        </Button>
      </div>
    </section>
  );
}
