import ListFilterBar from '@/components/list-filter-bar';
import { Button } from '@/components/ui/button';
import { useFetchChatAppList } from '@/hooks/chat-hooks';
import { Plus } from 'lucide-react';
import { ChatCard } from './chat-card';

export default function ChatList() {
  const { data: chatList } = useFetchChatAppList();

  return (
    <section className="p-8">
      <ListFilterBar title="Chat apps">
        <Button variant={'tertiary'} size={'sm'}>
          <Plus className="mr-2 h-4 w-4" />
          Create app
        </Button>
      </ListFilterBar>
      <div className="grid gap-6 sm:grid-cols-1 md:grid-cols-2 lg:grid-cols-4 xl:grid-cols-6 2xl:grid-cols-8">
        {chatList.map((x) => {
          return <ChatCard key={x.id} data={x}></ChatCard>;
        })}
      </div>
    </section>
  );
}
