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
import message from '@/components/ui/message';
import { useSetModalState } from '@/hooks/common-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { ReactFlowProvider } from '@xyflow/react';
import {
  ChevronDown,
  CirclePlay,
  History,
  LaptopMinimalCheck,
  Settings,
  Upload,
} from 'lucide-react';
import { ComponentPropsWithoutRef, useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import DataFlowCanvas from './canvas';
import { DropdownProvider } from './canvas/context';
import { Operator } from './constant';
import { LogContext } from './context';
import { useCancelCurrentDataflow } from './hooks/use-cancel-dataflow';
import { useHandleExportOrImportJsonFile } from './hooks/use-export-json';
import { useFetchDataOnMount } from './hooks/use-fetch-data';
import { useFetchLog } from './hooks/use-fetch-log';
import {
  useSaveGraph,
  useSaveGraphBeforeOpeningDebugDrawer,
  useWatchAgentChange,
} from './hooks/use-save-graph';
import { LogSheet } from './log-sheet';
import { SettingDialog } from './setting-dialog';
import useGraphStore from './store';
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
  const { handleExportJson } = useHandleExportOrImportJsonFile();
  const { saveGraph, loading } = useSaveGraph();
  const { flowDetail: agentDetail } = useFetchDataOnMount();
  const { handleRun, loading: running } =
    useSaveGraphBeforeOpeningDebugDrawer(showChatDrawer);

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

  const {
    visible: logSheetVisible,
    showModal: showLogSheet,
    hideModal: hideLogSheet,
  } = useSetModalState();

  const {
    isParsing,
    logs,
    messageId,
    setMessageId,
    isCompleted,
    stopFetchTrace,
    isLogEmpty,
  } = useFetchLog(logSheetVisible);

  const [uploadedFileData, setUploadedFileData] =
    useState<Record<string, any>>();
  const findNodeByName = useGraphStore((state) => state.findNodeByName);

  const handleRunAgent = useCallback(() => {
    if (!findNodeByName(Operator.Tokenizer)) {
      message.warning(t('dataflow.tokenizerRequired'));
      return;
    }

    if (isParsing) {
      // show log sheet
      showLogSheet();
    } else {
      hideLogSheet();
      handleRun();
    }
  }, [findNodeByName, handleRun, hideLogSheet, isParsing, showLogSheet, t]);

  const { handleCancel } = useCancelCurrentDataflow({
    messageId,
    stopFetchTrace,
  });

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
          <ButtonLoading
            variant={'secondary'}
            onClick={handleRunAgent}
            loading={running}
          >
            <CirclePlay className={isParsing ? 'animate-spin' : ''} />

            {isParsing || running ? t('dataflow.running') : t('flow.run')}
          </ButtonLoading>
          <Button variant={'secondary'} onClick={showVersionDialog}>
            <History />
            {t('flow.historyversion')}
          </Button>
          {/* <Button variant={'secondary'}>
            <Send />
            {t('flow.release')}
          </Button> */}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant={'secondary'}>
                <ChevronDown /> {t('flow.management')}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent>
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
      <LogContext.Provider
        value={{ messageId, setMessageId, setUploadedFileData }}
      >
        <ReactFlowProvider>
          <DropdownProvider>
            <DataFlowCanvas
              drawerVisible={chatDrawerVisible}
              hideDrawer={hideChatDrawer}
              showLogSheet={showLogSheet}
            ></DataFlowCanvas>
          </DropdownProvider>
        </ReactFlowProvider>
      </LogContext.Provider>
      {versionDialogVisible && (
        <DropdownProvider>
          <VersionDialog hideModal={hideVersionDialog}></VersionDialog>
        </DropdownProvider>
      )}
      {settingDialogVisible && (
        <SettingDialog hideModal={hideSettingDialog}></SettingDialog>
      )}
      {logSheetVisible && (
        <LogSheet
          hideModal={hideLogSheet}
          isParsing={isParsing}
          isCompleted={isCompleted}
          isLogEmpty={isLogEmpty}
          logs={logs}
          handleCancel={handleCancel}
          messageId={messageId}
          uploadedFileData={uploadedFileData}
        ></LogSheet>
      )}
    </section>
  );
}
