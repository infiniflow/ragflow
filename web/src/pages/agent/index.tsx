import EmbedDialog, {
  defaultWidgetSettings,
  type WidgetSettings,
} from '@/components/embed-dialog';
import { useShowEmbedModal } from '@/components/embed-dialog/use-show-embed-dialog';
import { PageHeader } from '@/components/page-header';
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
import { useFetchAgent, useSaveAgent } from '@/hooks/use-agent-request';
import { useFetchCanvasPresence } from '@/ee/hooks/use-agent-presence';
import { AgentEditGuard } from '@/ee/pages/agent/agent-edit-guard';
import { ReactFlowProvider } from '@xyflow/react';
import {
  ChevronDown,
  CirclePlay,
  History,
  LaptopMinimalCheck,
  MessageSquareCode,
  ScreenShare,
  Upload,
} from 'lucide-react';
import { ComponentPropsWithoutRef, useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';
import AgentCanvas from './canvas';
import { DropdownProvider } from './canvas/context';
import { CanvasChecklist } from './components/canvas-checklist';
import { PublishConfirmDialog } from './components/publish-confirm-dialog';
import { Operator } from './constant';
import { OwnerTenantIdContext } from './context';
import { GlobalParamSheet } from './gobal-variable-sheet';
import { useBuildDslData } from './hooks/use-build-dsl';
import { useCancelCurrentDataflow } from './hooks/use-cancel-dataflow';
import { useCanvasChecklist } from './hooks/use-canvas-checklist';
import { useHandleExportJsonFile } from './hooks/use-export-json';
import { useFetchPipelineLog } from './hooks/use-fetch-pipeline-log';
import { useIsPipeline } from './hooks/use-is-pipeline';
import { useIsWebhookMode } from './hooks/use-is-webhook';
import { useRunDataflow } from './hooks/use-run-dataflow';
import {
  useSaveGraph,
  useSaveGraphBeforeOpeningDebugDrawer,
  useWatchAgentChange,
} from './hooks/use-save-graph';
import { OnlineUsers } from '@/ee/pages/agent/online-users';
import { PipelineLogSheet } from './pipeline-log-sheet';
import PipelineRunSheet from './pipeline-run-sheet';
import useGraphStore from './store';
import { useAgentHistoryManager } from './use-agent-history-manager';
import { VersionDialog } from './version-dialog';
import WebhookSheet from './webhook-sheet';
import { RunTooltip } from './flow-tooltip';
import { debugRunLimitsTooltipKey } from './utils/debug-run-limits';

/**
 * Standardizes dropdown menu item styling for agent management actions.
 */
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

const AgentWidgetSettingsGlobalKey = 'sys.widget_settings';

/**
 * Displays the agent editor and persists agent-scoped widget defaults in DSL globals.
 */
export default function Agent() {
  const { id } = useParams();
  const isPipeline = useIsPipeline();
  const {
    visible: chatDrawerVisible,
    hideModal: hideChatDrawer,
    showModal: showChatDrawer,
  } = useSetModalState();
  const { t } = useTranslation();
  useAgentHistoryManager();
  // Resolves the i18n key for the canvas "Run" tooltip describing the
  // debug preview limits. It is shown ONLY for a dataflow (ingestion pipeline)
  // canvas — an agent canvas runs the agent, not an ingestion debug preview.
  const runTooltipKey = debugRunLimitsTooltipKey(isPipeline);
  // The ingestion (dataflow) canvas hides the Run/test-run entry entirely;
  // the agent canvas keeps its Run button.
  const showRunButton = !isPipeline;

  const { handleExportJson } = useHandleExportJsonFile();
  const { saveGraph, loading } = useSaveGraph();
  // Manual Save is a draft save, same as auto-save: it refreshes the live
  // canvas without minting a user_canvas_version row. Publishing does that.
  const handleSave = useCallback(async () => {
    await saveGraph(undefined, undefined, undefined, true);
  }, [saveGraph]);
  const handlePublish = useCallback(
    async (alias?: string) => {
      await saveGraph(undefined, undefined, true, undefined, alias);
    },
    [saveGraph],
  );
  const { data: agentDetail } = useFetchAgent();
  // The checklist displays all issues; Run separately checks blocking rules.
  const { issues } = useCanvasChecklist({
    ownerTenantId: agentDetail?.tenant_id,
    canvasID: id,
  });
  const { buildDslData } = useBuildDslData();
  const { saveAgent, loading: savingWidgetSettings } = useSaveAgent(false);
  const { handleRun } = useSaveGraphBeforeOpeningDebugDrawer(showChatDrawer);
  const {
    visible: versionDialogVisible,
    hideModal: hideVersionDialog,
    showModal: showVersionDialog,
  } = useSetModalState();

  const { showEmbedModal, hideEmbedModal, embedVisible, beta } =
    useShowEmbedModal();
  const time = useWatchAgentChange(chatDrawerVisible);
  const isWebhookMode = useIsWebhookMode();

  // pipeline

  const {
    visible: pipelineRunSheetVisible,
    hideModal: hidePipelineRunSheet,
    showModal: showPipelineRunSheet,
  } = useSetModalState();

  const {
    visible: webhookTestSheetVisible,
    hideModal: hideWebhookTestSheet,
    showModal: showWebhookTestSheet,
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

  const handleButtonRunClick = useCallback(async () => {
    if (isWebhookMode) {
      if ((await saveGraph())?.code === 0) showWebhookTestSheet();
    } else if (isPipeline) {
      handleRunPipeline();
    } else {
      handleRun();
    }
  }, [
    handleRun,
    handleRunPipeline,
    isPipeline,
    isWebhookMode,
    saveGraph,
    showWebhookTestSheet,
  ]);

  const handleEmbedClick = useCallback(() => {
    showEmbedModal();
  }, [showEmbedModal]);

  // Single source for the Run button so the tooltip gating in the JSX below
  // doesn't duplicate it.
  const runButton = (
    <Button
      data-testid="agent-run"
      variant={'secondary'}
      onClick={handleButtonRunClick}
    >
      <CirclePlay />
      {t('flow.run')}
    </Button>
  );

  const {
    run: runPipeline,
    loading: pipelineRunning,
    uploadedFileData,
  } = useRunDataflow({ showLogSheet: showPipelineLogSheet, setMessageId });

  const { data: presence } = useFetchCanvasPresence();
  const operatorPermission = presence.operator_permission ?? 0;
  const initialWidgetSettings = useMemo<WidgetSettings>(() => {
    const widgetSettings =
      agentDetail?.dsl?.globals?.[AgentWidgetSettingsGlobalKey];

    return {
      ...defaultWidgetSettings,
      ...(widgetSettings && typeof widgetSettings === 'object'
        ? widgetSettings
        : {}),
    };
  }, [agentDetail]);

  const handleSaveWidgetSettings = useCallback(
    async (widgetSettings: WidgetSettings) => {
      const dsl = buildDslData();

      return saveAgent({
        id: id!,
        title: agentDetail.title,
        dsl: {
          ...dsl,
          globals: {
            ...dsl.globals,
            [AgentWidgetSettingsGlobalKey]: widgetSettings,
          },
        },
      });
    },
    [agentDetail.title, buildDslData, id, saveAgent],
  );

  return (
    <section className="h-full" data-testid="agent-detail">
      <PageHeader className="pl-0">
        <section>
          <div className="text-xs text-text-secondary">
            {t('flow.autosaved')} {time}
          </div>
        </section>
        <div className="flex items-center gap-5">
          <OnlineUsers></OnlineUsers>
          <AgentEditGuard operatorPermission={operatorPermission}>
            <ButtonLoading
              variant={'secondary'}
              onClick={handleSave}
              loading={loading}
            >
              <LaptopMinimalCheck /> {t('flow.saveDraft')}
            </ButtonLoading>

            {showRunButton &&
              (runTooltipKey ? (
                <RunTooltip tooltip={runTooltipKey}>{runButton}</RunTooltip>
              ) : (
                runButton
              ))}
            {/* <PublishConfirmDialog
              agentDetail={agentDetail}
              loading={loading}
              onPublish={() => saveGraph(undefined, undefined, true)}
              onBeforeOpen={hasBlockingIssues}
            /> */}
            <PublishConfirmDialog
              agentDetail={agentDetail}
              loading={loading}
              onPublish={handlePublish}
            />
          </AgentEditGuard>
          <CanvasChecklist issues={issues} />
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant={'secondary'}>
                <ChevronDown /> {t('flow.management')}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent>
              <AgentEditGuard operatorPermission={operatorPermission}>
                <AgentDropdownMenuItem onClick={() => showGlobalParamSheet()}>
                  <MessageSquareCode />
                  {t('flow.conversationVariable')}
                </AgentDropdownMenuItem>
                <DropdownMenuSeparator />
              </AgentEditGuard>
              <AgentDropdownMenuItem onClick={showVersionDialog}>
                <History />
                {t('flow.historyVersion')}
              </AgentDropdownMenuItem>
              <DropdownMenuSeparator />
              <AgentDropdownMenuItem onClick={handleExportJson}>
                <Upload />
                {t('flow.export')}
              </AgentDropdownMenuItem>
              {isPipeline ||
                (location.hostname !== 'cloud.ragflow.io' && (
                  <>
                    <DropdownMenuSeparator />
                    <AgentDropdownMenuItem onClick={handleEmbedClick}>
                      <ScreenShare />
                      {t('common.embedIntoSite')}
                    </AgentDropdownMenuItem>
                  </>
                ))}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </PageHeader>
      <OwnerTenantIdContext.Provider value={agentDetail?.tenant_id}>
        <ReactFlowProvider>
          <DropdownProvider>
            <AgentCanvas
              drawerVisible={chatDrawerVisible}
              hideDrawer={hideChatDrawer}
            ></AgentCanvas>
          </DropdownProvider>
        </ReactFlowProvider>
      </OwnerTenantIdContext.Provider>
      {embedVisible && (
        <EmbedDialog
          visible={embedVisible}
          hideModal={hideEmbedModal}
          token={id!}
          from={SharedFrom.Agent}
          beta={beta}
          isAgent
          initialWidgetSettings={initialWidgetSettings}
          onSaveWidgetSettings={handleSaveWidgetSettings}
          savingWidgetSettings={savingWidgetSettings}
        ></EmbedDialog>
      )}
      {versionDialogVisible && (
        <DropdownProvider>
          <VersionDialog hideModal={hideVersionDialog}></VersionDialog>
        </DropdownProvider>
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
      {webhookTestSheetVisible && (
        <WebhookSheet hideModal={hideWebhookTestSheet}></WebhookSheet>
      )}
    </section>
  );
}
