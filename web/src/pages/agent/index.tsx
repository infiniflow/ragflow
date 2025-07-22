import { PageHeader } from '@/components/page-header';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
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
  LaptopMinimalCheck,
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
import { useGetBeginNodeDataInputs } from './hooks/use-get-begin-query';
import { useOpenDocument } from './hooks/use-open-document';
import {
  useSaveGraph,
  useSaveGraphBeforeOpeningDebugDrawer,
} from './hooks/use-save-graph';
import { useShowEmbedModal } from './hooks/use-show-dialog';
import { UploadAgentDialog } from './upload-agent-dialog';
import { VersionDialog } from './version-dialog';

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
  const inputs = useGetBeginNodeDataInputs();
  const { handleRun } = useSaveGraphBeforeOpeningDebugDrawer(showChatDrawer);
  const handleRunAgent = useCallback(() => {
    if (inputs.length > 0) {
      showChatDrawer();
    } else {
      handleRun();
    }
  }, [handleRun, inputs, showChatDrawer]);
  const {
    visible: versionDialogVisible,
    hideModal: hideVersionDialog,
    showModal: showVersionDialog,
  } = useSetModalState();

  const { showEmbedModal, hideEmbedModal, embedVisible, beta } =
    useShowEmbedModal();

  return (
    <section className="h-full">
      <PageHeader>
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink onClick={navigateToAgentList}>
                Agent
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>{flowDetail.title}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
        <div className="flex items-center gap-5">
          <ButtonLoading
            variant={'secondary'}
            onClick={() => saveGraph()}
            loading={loading}
          >
            <LaptopMinimalCheck /> {t('flow.save')}
          </ButtonLoading>
          <Button variant={'secondary'} onClick={handleRunAgent}>
            <CirclePlay />
            {t('flow.run')}
          </Button>
          <Button variant={'secondary'} onClick={showVersionDialog}>
            <History />
            {t('flow.historyversion')}
          </Button>
          <Button variant={'secondary'}>
            <Logs />
            {t('flow.log')}
          </Button>

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant={'secondary'}>
                <ChevronDown /> {t('flow.management')}
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
                {t('flow.import')}
              </AgentDropdownMenuItem>
              <DropdownMenuSeparator />
              <AgentDropdownMenuItem onClick={handleExportJson}>
                <Upload />
                {t('flow.export')}
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
          from={SharedFrom.Agent}
          beta={beta}
          isAgent
        ></EmbedDialog>
      )}
      {versionDialogVisible && (
        <VersionDialog hideModal={hideVersionDialog}></VersionDialog>
      )}
    </section>
  );
}
