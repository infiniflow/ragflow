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
import { useSetModalState } from '@/hooks/common-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { ReactFlowProvider } from '@xyflow/react';
import {
  ChevronDown,
  CirclePlay,
  Download,
  History,
  LaptopMinimalCheck,
  Settings,
  Upload,
} from 'lucide-react';
import { ComponentPropsWithoutRef, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import DataFlowCanvas from './canvas';
import { DropdownProvider } from './canvas/context';
import { useHandleExportOrImportJsonFile } from './hooks/use-export-json';
import { useFetchDataOnMount } from './hooks/use-fetch-data';
import { useGetBeginNodeDataInputs } from './hooks/use-get-begin-query';
import {
  useSaveGraph,
  useSaveGraphBeforeOpeningDebugDrawer,
  useWatchAgentChange,
} from './hooks/use-save-graph';
import { SettingDialog } from './setting-dialog';
import { UploadAgentDialog } from './upload-agent-dialog';
import { useAgentHistoryManager } from './use-agent-history-manager';
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

export default function DataFlow() {
  const { navigateToAgents } = useNavigatePage();
  const {
    visible: chatDrawerVisible,
    hideModal: hideChatDrawer,
    showModal: showChatDrawer,
  } = useSetModalState();
  const { t } = useTranslation();
  useAgentHistoryManager();
  const {
    handleExportJson,
    handleImportJson,
    fileUploadVisible,
    onFileUploadOk,
    hideFileUploadModal,
  } = useHandleExportOrImportJsonFile();
  const { saveGraph, loading } = useSaveGraph();
  const { flowDetail: agentDetail } = useFetchDataOnMount();
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

  const {
    visible: settingDialogVisible,
    hideModal: hideSettingDialog,
    showModal: showSettingDialog,
  } = useSetModalState();

  const time = useWatchAgentChange(chatDrawerVisible);

  return (
    <section className="h-full">
      <PageHeader>
        <section>
          <Breadcrumb>
            <BreadcrumbList>
              <BreadcrumbItem>
                <BreadcrumbLink onClick={navigateToAgents}>
                  Agent
                </BreadcrumbLink>
              </BreadcrumbItem>
              <BreadcrumbSeparator />
              <BreadcrumbItem>
                <BreadcrumbPage>{agentDetail.title}</BreadcrumbPage>
              </BreadcrumbItem>
            </BreadcrumbList>
          </Breadcrumb>
          <div className="text-xs text-text-secondary translate-y-3">
            {t('flow.autosaved')} {time}
          </div>
        </section>
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

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant={'secondary'}>
                <ChevronDown /> {t('flow.management')}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent>
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
              <AgentDropdownMenuItem onClick={showSettingDialog}>
                <Settings />
                {t('flow.setting')}
              </AgentDropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </PageHeader>
      <ReactFlowProvider>
        <DropdownProvider>
          <DataFlowCanvas
            drawerVisible={chatDrawerVisible}
            hideDrawer={hideChatDrawer}
          ></DataFlowCanvas>
        </DropdownProvider>
      </ReactFlowProvider>
      {fileUploadVisible && (
        <UploadAgentDialog
          hideModal={hideFileUploadModal}
          onOk={onFileUploadOk}
        ></UploadAgentDialog>
      )}

      {versionDialogVisible && (
        <DropdownProvider>
          <VersionDialog hideModal={hideVersionDialog}></VersionDialog>
        </DropdownProvider>
      )}
      {settingDialogVisible && (
        <SettingDialog hideModal={hideSettingDialog}></SettingDialog>
      )}
    </section>
  );
}
