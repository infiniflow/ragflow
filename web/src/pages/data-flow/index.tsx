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
  History,
  LaptopMinimalCheck,
  Settings,
  Upload,
} from 'lucide-react';
import { ComponentPropsWithoutRef, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import DataFlowCanvas from './canvas';
import { DropdownProvider } from './canvas/context';
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
  const { handleRun } = useSaveGraphBeforeOpeningDebugDrawer(showChatDrawer);

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

  const { isParsing, data, messageId, setMessageId } = useFetchLog();

  const handleRunAgent = useCallback(() => {
    if (isParsing) {
      // show log sheet
      showLogSheet();
    } else {
      handleRun();
    }
  }, [handleRun, isParsing, showLogSheet]);

  const { handleCancel } = useCancelCurrentDataflow({
    messageId,
    setMessageId,
    hideLogSheet,
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
          <Button
            variant={'secondary'}
            onClick={handleRunAgent}
            disabled={isParsing}
          >
            <CirclePlay className={isParsing ? 'animate-spin' : ''} />
            {isParsing ? t('dataflow.running') : t('flow.run')}
          </Button>
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
      <LogContext.Provider value={{ messageId, setMessageId }}>
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
          logs={data}
          handleCancel={handleCancel}
        ></LogSheet>
      )}
    </section>
  );
}
