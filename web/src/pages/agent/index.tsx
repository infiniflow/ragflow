import { PageHeader } from '@/components/page-header';
import { Button, ButtonLoading } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { SharedFrom } from '@/constants/chat';
import { useSetModalState } from '@/hooks/common-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { ReactFlowProvider } from '@xyflow/react';
import {
  ChevronDown,
  CirclePlay,
  Download,
  History,
  Key,
  Logs,
  ScreenShare,
  Upload,
} from 'lucide-react';
import { ComponentPropsWithoutRef, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'umi';
import AgentCanvas from './canvas';
import EmbedDialog from './embed-dialog';
import { useHandleExportOrImportJsonFile } from './hooks/use-export-json';
import { useFetchDataOnMount } from './hooks/use-fetch-data';
import { useGetBeginNodeDataQuery } from './hooks/use-get-begin-query';
import { useOpenDocument } from './hooks/use-open-document';
import {
  useSaveGraph,
  useSaveGraphBeforeOpeningDebugDrawer,
} from './hooks/use-save-graph';
import { useShowEmbedModal } from './hooks/use-show-dialog';
import { BeginQuery } from './interface';
import { UploadAgentDialog } from './upload-agent-dialog';

function AgentDropdownMenuItem({
  children,
  ...props
}: ComponentPropsWithoutRef<typeof DropdownMenuItem>) {
  return (
    <DropdownMenuItem className="justify-start" {...props}>
      {children}
    </DropdownMenuItem>
  );
}

export default function Agent() {
  const { id } = useParams();
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

  const { showEmbedModal, hideEmbedModal, embedVisible, beta } =
    useShowEmbedModal();

  return (
    <section className="h-full">
      <PageHeader back={navigateToAgentList} title={flowDetail.title}>
        <div className="flex items-center gap-2">
          <ButtonLoading
            variant={'secondary'}
            onClick={() => saveGraph()}
            loading={loading}
          >
            Save
          </ButtonLoading>
          <Button variant={'secondary'} onClick={handleRunAgent}>
            <CirclePlay />
            Run app
          </Button>
          <Button variant={'secondary'}>
            <History />
            History version
          </Button>
          <Button variant={'secondary'}>
            <Logs />
            Log
          </Button>

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant={'secondary'}>
                <ChevronDown /> Management
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent>
              <AgentDropdownMenuItem onClick={openDocument}>
                <Key />
                API
              </AgentDropdownMenuItem>
              <DropdownMenuSeparator />
              <AgentDropdownMenuItem onClick={handleImportJson}>
                <Download />
                Import
              </AgentDropdownMenuItem>
              <DropdownMenuSeparator />
              <AgentDropdownMenuItem onClick={handleExportJson}>
                <Upload />
                Export
              </AgentDropdownMenuItem>
              <DropdownMenuSeparator />
              <AgentDropdownMenuItem onClick={showEmbedModal}>
                <ScreenShare />
                {t('common.embedIntoSite')}
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
      {embedVisible && (
        <EmbedDialog
          visible={embedVisible}
          hideModal={hideEmbedModal}
          token={id!}
          form={SharedFrom.Agent}
          beta={beta}
          isAgent
        ></EmbedDialog>
      )}
    </section>
  );
}
