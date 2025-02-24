import { PageHeader } from '@/components/page-header';
import { Button } from '@/components/ui/button';
import { SidebarProvider, SidebarTrigger } from '@/components/ui/sidebar';
import { useSetModalState } from '@/hooks/common-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { Trash2 } from 'lucide-react';
import { AgentSidebar } from './agent-sidebar';
import FlowCanvas from './canvas';
import { useFetchDataOnMount } from './hooks/use-fetch-data';

export default function Agent() {
  const { navigateToAgentList } = useNavigatePage();
  const {
    visible: chatDrawerVisible,
    hideModal: hideChatDrawer,
    showModal: showChatDrawer,
  } = useSetModalState();

  useFetchDataOnMount();

  return (
    <section>
      <PageHeader back={navigateToAgentList} title="Agent 01">
        <div className="flex items-center gap-2">
          <Button variant={'icon'} size={'icon'}>
            <Trash2 />
          </Button>
          <Button variant={'outline'} size={'sm'}>
            Save
          </Button>
          <Button variant={'outline'} size={'sm'}>
            API
          </Button>
          <Button variant={'outline'} size={'sm'}>
            Run app
          </Button>

          <Button variant={'tertiary'} size={'sm'}>
            Publish
          </Button>
        </div>
      </PageHeader>
      <div>
        <SidebarProvider>
          <AgentSidebar />
          <div className="w-full">
            <SidebarTrigger />
            <div className="w-full h-full">
              <FlowCanvas
                drawerVisible={chatDrawerVisible}
                hideDrawer={hideChatDrawer}
              ></FlowCanvas>
            </div>
          </div>
        </SidebarProvider>
      </div>
    </section>
  );
}
