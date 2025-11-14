import EmbedDialog from '@/components/embed-dialog';
import { useShowEmbedModal } from '@/components/embed-dialog/use-show-embed-dialog';
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
import { SharedFrom } from '@/constants/chat';
import { useSetModalState } from '@/hooks/common-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { ReactFlowProvider } from '@xyflow/react';
import {
  ChevronDown,
  CirclePlay,
  History,
  LaptopMinimalCheck,
  Logs,
  MessageSquareCode,
  ScreenShare,
  Settings,
  Upload,
} from 'lucide-react';
import { ComponentPropsWithoutRef, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'umi';
import AgentCanvas from './canvas';
import { DropdownProvider } from './canvas/context';
import { Operator } from './constant';
import { GlobalParamSheet } from './gobal-variable-sheet';
import { useCancelCurrentDataflow } from './hooks/use-cancel-dataflow';
import { useHandleExportJsonFile } from './hooks/use-export-json';
import { useFetchDataOnMount } from './hooks/use-fetch-data';
import { useFetchPipelineLog } from './hooks/use-fetch-pipeline-log';
import { useGetBeginNodeDataInputs } from './hooks/use-get-begin-query';
import { useIsPipeline } from './hooks/use-is-pipeline';
import { useRunDataflow } from './hooks/use-run-dataflow';
import {
  useSaveGraph,
  useSaveGraphBeforeOpeningDebugDrawer,
  useWatchAgentChange,
} from './hooks/use-save-graph';
import { PipelineLogSheet } from './pipeline-log-sheet';
import PipelineRunSheet from './pipeline-run-sheet';
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

export default function Agent() {
  const { id } = useParams();
  const isPipeline = useIsPipeline();
  const { navigateToAgents } = useNavigatePage();
  const {
    visible: chatDrawerVisible,
    hideModal: hideChatDrawer,
    showModal: showChatDrawer,
  } = useSetModalState();
  const { t } = useTranslation();
  useAgentHistoryManager();

  const { handleExportJson } = useHandleExportJsonFile();
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

  const { showEmbedModal, hideEmbedModal, embedVisible, beta } =
    useShowEmbedModal();
  const { navigateToAgentLogs } = useNavigatePage();
  const time = useWatchAgentChange(chatDrawerVisible);

  // pipeline

  const {
    visible: pipelineRunSheetVisible,
    hideModal: hidePipelineRunSheet,
    showModal: showPipelineRunSheet,
  } = useSetModalState();

  const {
    visible: pipelineLogSheetVisible,
    showModal: showPipelineLogSheet,
    hideModal: hidePipelineLogSheet,
  } = useSetModalState();

  const {
    visible: globalParamSheetVisible,
    showModal: showGlobalParamSheet,
    hideModal: hideGlobalParamSheet,
  } = useSetModalState();

  const {
    isParsing,
    logs,
    messageId,
    setMessageId,
    isCompleted,
    stopFetchTrace,
    isLogEmpty,
  } = useFetchPipelineLog(pipelineLogSheetVisible);

  const findNodeByName = useGraphStore((state) => state.findNodeByName);

  const handleRunPipeline = useCallback(() => {
    if (!findNodeByName(Operator.Tokenizer)) {
      message.warning(t('flow.tokenizerRequired'));
      return;
    }

    if (isParsing) {
      // show log sheet
      showPipelineLogSheet();
    } else {
      hidePipelineLogSheet();
      // handleRun();
      showPipelineRunSheet();
    }
  }, [
    findNodeByName,
    hidePipelineLogSheet,
    isParsing,
    showPipelineLogSheet,
    showPipelineRunSheet,
    t,
  ]);

  const { handleCancel } = useCancelCurrentDataflow({
    messageId,
    stopFetchTrace,
  });

  const handleButtonRunClick = useCallback(() => {
    if (isPipeline) {
      handleRunPipeline();
    } else {
      handleRunAgent();
    }
  }, [handleRunAgent, handleRunPipeline, isPipeline]);

  const {
    run: runPipeline,
    loading: pipelineRunning,
    uploadedFileData,
  } = useRunDataflow({ showLogSheet: showPipelineLogSheet, setMessageId });

  return (
    <section className="h-full">
      <PageHeader>
        <section>
          <Breadcrumb>
            <BreadcrumbList>
              <BreadcrumbItem>
                <BreadcrumbLink onClick={navigateToAgents}>
                  {t('header.flow')}
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
            onClick={() => showGlobalParamSheet()}
            loading={loading}
          >
            <MessageSquareCode /> {t('flow.conversationVariable')}
          </ButtonLoading>
          <Button variant={'secondary'} onClick={handleButtonRunClick}>
            <CirclePlay />
            {t('flow.run')}
          </Button>
          <Button variant={'secondary'} onClick={showVersionDialog}>
            <History />
            {t('flow.historyversion')}
          </Button>
          {isPipeline || (
            <Button
              variant={'secondary'}
              onClick={navigateToAgentLogs(id as string)}
            >
              <Logs />
              {t('flow.log')}
            </Button>
          )}
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
              {isPipeline ||
                (location.hostname !== 'demo.ragflow.io' && (
                  <>
                    <DropdownMenuSeparator />
                    <AgentDropdownMenuItem onClick={showEmbedModal}>
                      <ScreenShare />
                      {t('common.embedIntoSite')}
                    </AgentDropdownMenuItem>
                  </>
                ))}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </PageHeader>
      <ReactFlowProvider>
        <DropdownProvider>
          <AgentCanvas
            drawerVisible={chatDrawerVisible}
            hideDrawer={hideChatDrawer}
          ></AgentCanvas>
        </DropdownProvider>
      </ReactFlowProvider>
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
        <DropdownProvider>
          <VersionDialog hideModal={hideVersionDialog}></VersionDialog>
        </DropdownProvider>
      )}
      {settingDialogVisible && (
        <SettingDialog hideModal={hideSettingDialog}></SettingDialog>
      )}

      {pipelineLogSheetVisible && (
        <PipelineLogSheet
          hideModal={hidePipelineLogSheet}
          isParsing={isParsing}
          isCompleted={isCompleted}
          isLogEmpty={isLogEmpty}
          logs={logs}
          handleCancel={handleCancel}
          messageId={messageId}
          uploadedFileData={uploadedFileData}
        ></PipelineLogSheet>
      )}
      {pipelineRunSheetVisible && (
        <PipelineRunSheet
          hideModal={hidePipelineRunSheet}
          run={runPipeline}
          loading={pipelineRunning}
        ></PipelineRunSheet>
      )}
      {globalParamSheetVisible && (
        <GlobalParamSheet
          data={{}}
          hideModal={hideGlobalParamSheet}
        ></GlobalParamSheet>
      )}
    </section>
  );
}
