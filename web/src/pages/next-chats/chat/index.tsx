import { PageHeader } from '@/components/page-header';
import { Button } from '@/components/ui/button';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { EllipsisVertical } from 'lucide-react';
import { AppSettings } from './app-settings';
import { ChatBox } from './chat-box';
import { Sessions } from './sessions';

export default function Chat() {
  const { navigateToChatList } = useNavigatePage();

  return (
    <section className="h-full flex flex-col">
      <PageHeader back={navigateToChatList} title="Chat app 01">
        <div className="flex items-center gap-2">
          <Button variant={'icon'} size={'icon'}>
            <EllipsisVertical />
          </Button>
          <Button variant={'tertiary'} size={'sm'}>
            Publish
          </Button>
        </div>
      </PageHeader>
      <div className="flex flex-1">
        <Sessions></Sessions>
        <ChatBox></ChatBox>
        <AppSettings></AppSettings>
      </div>
    </section>
  );
}
