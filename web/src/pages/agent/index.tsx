import { PageHeader } from '@/components/page-header';
import { Button, ButtonLoading } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useSetModalState } from '@/hooks/common-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { ReactFlowProvider } from '@xyflow/react';
import { CodeXml, EllipsisVertical, Forward, Import, Key } from 'lucide-react';
import { ComponentPropsWithoutRef, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import AgentCanvas from './canvas';
import { useHandleExportOrImportJsonFile } from './hooks/use-export-json';
import { useFetchDataOnMount } from './hooks/use-fetch-data';
import { useGetBeginNodeDataQuery } from './hooks/use-get-begin-query';
import { useOpenDocument } from './hooks/use-open-document';
import {
  useSaveGraph,
  useSaveGraphBeforeOpeningDebugDrawer,
} from './hooks/use-save-graph';
import { BeginQuery } from './interface';
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
  const { saveGraph, loading } = useSaveGraph();

  const { flowDetail } = useFetchDataOnMount();
  const getBeginNodeDataQuery = useGetBeginNodeDataQuery();

  const { handleRun } = useSaveGraphBeforeOpeningDebugDrawer(showChatDrawer);

  const handleRunAgent = useCallback(() => {
    const query: BeginQuery[] = getBeginNodeDataQuery();
    if (query.length > 0) {
      showChatDrawer();
    } else {
      handleRun();
    }
  }, [getBeginNodeDataQuery, handleRun, showChatDrawer]);

  return (
    <section className="h-full">
      <PageHeader back={navigateToAgentList} title={flowDetail.title}>
        <div className="flex items-center gap-2">
          <ButtonLoading
            variant={'outline'}
            onClick={() => saveGraph()}
            loading={loading}
          >
            Save
          </ButtonLoading>
          <Button variant={'outline'} onClick={handleRunAgent}>
            Run app
          </Button>
          <Button variant={'outline'}>Publish</Button>

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
        </div>
      </PageHeader>
      <ReactFlowProvider>
        <AgentCanvas
          drawerVisible={chatDrawerVisible}
          hideDrawer={hideChatDrawer}
        ></AgentCanvas>
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
