import { PageHeader } from '@/components/page-header';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { SidebarProvider, SidebarTrigger } from '@/components/ui/sidebar';
import { useSetModalState } from '@/hooks/common-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { ReactFlowProvider } from '@xyflow/react';
import { CodeXml, EllipsisVertical, Forward, Import, Key } from 'lucide-react';
import { ComponentPropsWithoutRef } from 'react';
import { useTranslation } from 'react-i18next';
import { AgentSidebar } from './agent-sidebar';
import FlowCanvas from './canvas';
import { useHandleExportOrImportJsonFile } from './hooks/use-export-json';
import { useFetchDataOnMount } from './hooks/use-fetch-data';
import { useOpenDocument } from './hooks/use-open-document';
import { UploadAgentDialog } from './upload-agent-dialog';

function AgentDropdownMenuItem({
  children,
  ...props
}: ComponentPropsWithoutRef<typeof DropdownMenuItem>) {
  return (
    <DropdownMenuItem className="flex justify-between items-center" {...props}>
      {children}
    </DropdownMenuItem>
  );
}

export default function Agent() {
  const { navigateToAgentList } = useNavigatePage();
  const {
    visible: chatDrawerVisible,
    hideModal: hideChatDrawer,
    showModal: showChatDrawer,
  } = useSetModalState();
  const { t } = useTranslation();
  const openDocument = useOpenDocument();
  const {
    handleExportJson,
    handleImportJson,
    fileUploadVisible,
    onFileUploadOk,
    hideFileUploadModal,
  } = useHandleExportOrImportJsonFile();

  const { flowDetail } = useFetchDataOnMount();

  return (
    <section>
      <PageHeader back={navigateToAgentList} title={flowDetail.title}>
        <div className="flex items-center gap-2">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant={'icon'} size={'icon'}>
                <EllipsisVertical />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent>
              <AgentDropdownMenuItem onClick={openDocument}>
                API
                <Key />
              </AgentDropdownMenuItem>
              <DropdownMenuSeparator />
              <AgentDropdownMenuItem onClick={handleImportJson}>
                Import
                <Import />
              </AgentDropdownMenuItem>
              <DropdownMenuSeparator />
              <AgentDropdownMenuItem onClick={handleExportJson}>
                Export
                <Forward />
              </AgentDropdownMenuItem>
              <DropdownMenuSeparator />
              <AgentDropdownMenuItem>
                {t('common.embedIntoSite')}
                <CodeXml />
              </AgentDropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>

          <Button variant={'outline'} size={'sm'}>
            Save
          </Button>
          <Button variant={'outline'} size={'sm'}>
            Run app
          </Button>

          <Button variant={'tertiary'} size={'sm'}>
            Publish
          </Button>
        </div>
      </PageHeader>
      <ReactFlowProvider>
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
      </ReactFlowProvider>
      {fileUploadVisible && (
        <UploadAgentDialog
          hideModal={hideFileUploadModal}
          onOk={onFileUploadOk}
        ></UploadAgentDialog>
      )}
    </section>
  );
}
