import { MoreButton } from '@/components/more-button';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Card, CardContent } from '@/components/ui/card';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { IDialog } from '@/interfaces/database/chat';
import { formatDate } from '@/utils/date';
import { ChatDropdown } from './chat-dropdown';
import { useRenameChat } from './hooks/use-rename-chat';

export type IProps = {
  data: IDialog;
} & Pick<ReturnType<typeof useRenameChat>, 'showChatRenameModal'>;

export function ChatCard({ data, showChatRenameModal }: IProps) {
  const { navigateToChat } = useNavigatePage();

  return (
    <Card key={data.id} className="w-40" onClick={navigateToChat(data.id)}>
      <CardContent className="p-2.5 pt-2 group">
        <section className="flex justify-between mb-2">
          <div className="flex gap-2 items-center">
            <RAGFlowAvatar
              className="size-6 rounded-lg"
              avatar={data.icon}
              name={data.name || 'CN'}
            ></RAGFlowAvatar>
          </div>
          <ChatDropdown chat={data} showChatRenameModal={showChatRenameModal}>
            <MoreButton></MoreButton>
          </ChatDropdown>
        </section>
        <div className="flex justify-between items-end">
          <div className="w-full">
            <h3 className="text-lg font-semibold mb-2 line-clamp-1">
              {data.name}
            </h3>
            <p className="text-xs text-text-sub-title">{data.description}</p>
            <p className="text-xs text-text-sub-title">
              {formatDate(data.update_time)}
            </p>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
