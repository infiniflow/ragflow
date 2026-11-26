import { IconFontFill } from '@/components/icon-font';
import { ParserUnavailableLabel } from '@/components/parser-unavailable-label';
import { PipelineVersionLabel } from '@/components/pipeline-version-label';
import { Button } from '@/components/ui/button';
import message from '@/components/ui/message';
import { Progress } from '@/components/ui/progress';
import { Separator } from '@/components/ui/separator';
import { RAGFlowTooltip } from '@/components/ui/tooltip';
import { ParsingErrorLabel } from '@/ee/pages/dataset/dataset/parsing-error-label';
import { IDocumentInfo } from '@/interfaces/database/document';
import { hasReadPermission } from '@/ee/utils/permission-util';
import { cn } from '@/lib/utils';
import {
  CircleQuestionMark,
  CircleX,
  Clock3,
  Loader2,
  TriangleAlert,
} from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { DocumentType, RunningStatus } from './constant';
import { ParsingCard } from './parsing-card';
import { ReparseDialog } from './reparse-dialog';
import { UseChangeDocumentParserShowType } from './use-change-document-parser';
import { useDocumentPipelineVersion } from './use-document-pipeline-version';
import { useHandleRunDocumentByIds } from './use-run-document';
import {
  getDocumentParserLabel,
  getDocumentRunningStatus,
  isDocumentProcessing,
  isDocumentStopping,
} from './utils';

const IconMap = {
  [RunningStatus.UNSTART]: (
    <IconFontFill name="play" className="text-accent-primary size-4" />
  ),
  [RunningStatus.RUNNING]: <CircleX className="size-4 text-state-error" />,
  [RunningStatus.CANCEL]: (
    <IconFontFill name="reparse" className="text-accent-primary size-4" />
  ),
  [RunningStatus.DONE]: (
    <IconFontFill name="reparse" className="text-accent-primary size-4" />
  ),
  [RunningStatus.FAIL]: (
    <IconFontFill name="reparse" className="text-accent-primary size-4" />
  ),
  [RunningStatus.SCHEDULE]: (
    <IconFontFill name="reparse" className="text-accent-primary size-4" />
  ),
  // Status renders QUEUED as a dedicated clock chip; this key only keeps the
  // icon map type-exhaustive.
  [RunningStatus.QUEUED]: (
    <IconFontFill name="play" className="text-accent-primary size-4" />
  ),
};

const ParseStatusStateMap = {
  [RunningStatus.UNSTART]: 'unstart',
  [RunningStatus.RUNNING]: 'running',
  [RunningStatus.CANCEL]: 'cancel',
  [RunningStatus.DONE]: 'success',
  [RunningStatus.FAIL]: 'fail',
  [RunningStatus.SCHEDULE]: 'running',
} as const;

export function ParseDropdownButton({
  record,
  showChangeParserModal,
}: {
  record: IDocumentInfo;
} & UseChangeDocumentParserShowType) {
  const { t } = useTranslation();
  const parserLabel = getDocumentParserLabel(record);
  const { version, missing: parserMissing } =
    useDocumentPipelineVersion(record);
  const isPipeline = !!record.pipeline_id;

  const handleShowChangeParserModal = useCallback(() => {
    showChangeParserModal(record);
  }, [record, showChangeParserModal]);

  // Pipeline documents are labeled by the version they parse with — the
  // version title plus its alias as secondary text, the same display the
  // parser tree select uses. While the version list is still loading, fall
  // back to the pipeline name. A binding whose pipeline or pinned version was
  // deleted renders the same "unavailable" display as ParserTreeSelect's
  // missing value, with the reason on hover. Built-in parsers keep the plain
  // parser label.
  const versionTitle = version?.title ?? parserLabel;

  const tooltipContent = parserMissing ? (
    <p>{t('knowledgeConfiguration.parserOptionUnavailableMessage')}</p>
  ) : isPipeline ? (
    <div>
      <p>{versionTitle}</p>
      {version?.alias ? <p>{version.alias}</p> : null}
    </div>
  ) : (
    <p>{parserLabel}</p>
  );

  return (
    <div className="flex items-center gap-1">
      <RAGFlowTooltip asChild tooltip={tooltipContent}>
        <Button
          variant="static"
          size="auto"
          disabled={!hasReadPermission(record.operator_permission ?? 0)}
          onClick={handleShowChangeParserModal}
        >
          {parserMissing ? (
            <ParserUnavailableLabel className="text-text-secondary" />
          ) : isPipeline ? (
            <PipelineVersionLabel
              title={versionTitle}
              alias={version?.alias}
              className="max-w-72"
              aliasClassName="max-w-[50%]"
            />
          ) : (
            parserLabel
          )}
        </Button>
      </RAGFlowTooltip>
      {record.pipeline_version_outdated && !parserMissing ? (
        <RAGFlowTooltip tooltip={t('flow.version.outdated')}>
          <span className="text-destructive cursor-help font-bold">!</span>
        </RAGFlowTooltip>
      ) : null}
    </div>
  );
}

export function ParseCell({
  record,
  showChangeParserModal,
  documentEditButtonDisabled = false,
}: {
  record: IDocumentInfo;
  documentEditButtonDisabled?: boolean;
} & UseChangeDocumentParserShowType) {
  const { t } = useTranslation();
  const { chunk_count, id } = record;
  const effectiveRun = getDocumentRunningStatus(record);
  const {
    handleRunDocumentByIds,
    loading: isRunLoading,
    visible: reparseDialogVisible,
    showModal: showReparseDialogModal,
    hideModal: hideReparseDialogModal,
  } = useHandleRunDocumentByIds(id, showChangeParserModal);
  const isRunning = isDocumentProcessing(record);
  const operationIcon = isRunning
    ? IconMap[RunningStatus.RUNNING]
    : IconMap[effectiveRun];
  const isStopping = isDocumentStopping(record);
  const isZeroChunk = chunk_count === 0;
  const { missing: parserMissing } = useDocumentPipelineVersion(record);
  const showParse = record.type !== DocumentType.Virtual;

  const handleOperationIconClick = (option?: {
    delete: boolean;
    apply_kb: boolean;
  }) => {
    if (documentEditButtonDisabled) {
      return;
    }
    handleRunDocumentByIds(record, isRunning, option);
  };

  // Existing chunks are always dropped server-side on re-ingest, so the
  // confirmation is only needed when there are existing chunks; otherwise, and
  // always when cancelling a run, the action fires straight away.
  const needsParseConfirm = !isRunning && !isZeroChunk;

  const handleParseClick = () => {
    // A document whose pipeline or pinned version was deleted can no longer
    // parse — warn and keep the run request from firing (and the reparse
    // confirmation from opening). Cancelling a running parse stays allowed:
    // it never touches the pipeline.
    if (!isRunning && parserMissing) {
      message.warning(
        t('knowledgeConfiguration.parserOptionUnavailableMessage'),
      );
      return;
    }
    if (needsParseConfirm) {
      showReparseDialogModal();
      return;
    }
    handleOperationIconClick();
  };

  return (
    <section className="flex w-full items-center gap-2">
      <ParseDropdownButton
        record={record}
        showChangeParserModal={showChangeParserModal}
      />
      {showParse && (
        <>
          <Separator orientation="vertical" className="ml-auto h-4 shrink-0" />
          {isRunLoading && !isRunning ? (
            <Button size="icon-xs" variant="ghost" disabled>
              <Loader2 className="size-4 animate-spin" />
            </Button>
          ) : (
            <Button
              variant="ghost"
              size="icon-xs"
              disabled={isStopping || documentEditButtonDisabled}
              onClick={handleParseClick}
              data-testid="document-parse-toggle"
            >
              {operationIcon}
            </Button>
          )}
        </>
      )}
      {reparseDialogVisible && (
        <ReparseDialog
          handleOperationIconClick={handleOperationIconClick}
          visible={reparseDialogVisible}
          hideModal={hideReparseDialogModal}
        />
      )}
    </section>
  );
}

export function ParsingStatusCell({
  record,
  showLog,
}: {
  record: IDocumentInfo;
  showLog: (record: IDocumentInfo) => void;
}) {
  const { t } = useTranslation();
  const { progress } = record;
  const effectiveRun = getDocumentRunningStatus(record);
  const p = Number((progress * 100).toFixed(2));
  const isRunning = isDocumentProcessing(record);
  const isQueued = effectiveRun === RunningStatus.QUEUED;
  const isStopping = isDocumentStopping(record);
  const showParse = record.type !== DocumentType.Virtual;
  const canReadDocument = hasReadPermission(record.operator_permission ?? 0);

  const handleShowLog = useCallback(() => {
    showLog(record);
  }, [record, showLog]);

  if (!showParse) {
    return null;
  }

  return (
    <section
      className="flex items-center"
      data-testid="document-parse-status"
      data-state={
        isQueued
          ? 'queued'
          : isStopping
            ? 'stopping'
            : (ParseStatusStateMap[effectiveRun] ?? 'unknown')
      }
    >
      {isRunning ? (
        <div className="relative">
          {/* While STOPPING the underlying running row stays visible
              but reads as disabled: dimmed, non-interactive, with the
              status button disabled. The scrim on top carries the spinner. */}
          <div
            data-testid="document-processing-row"
            data-stopping={isStopping || undefined}
            className={cn(
              'flex items-center gap-2',
              isStopping && 'pointer-events-none opacity-50',
            )}
          >
            {isQueued ? (
              <Button
                size="auto"
                variant="static"
                disabled={isStopping || !canReadDocument}
                onClick={handleShowLog}
              >
                <Clock3 className="size-4" />
                {t('knowledgeDetails.runningStatusQueued')}
              </Button>
            ) : (
              <Button
                size="auto"
                variant="static"
                disabled={isStopping || !canReadDocument}
                onClick={handleShowLog}
              >
                <Progress value={p} className="h-1 flex-1 min-w-10" />
                <div className="flex items-center gap-1">
                  {p}%
                  <span className="inline-flex items-center">
                    <CircleQuestionMark className="size-4" />
                  </span>
                </div>
              </Button>
            )}
          </div>

          {/* STOPPING means a cancel request is in flight but the worker has
              not reached a terminal state yet. The translucent scrim keeps the
              dimmed running row visible while the spinner marks the in-flight
              cancel, until the next poll observes STOPPED/FAILED. */}
          {isStopping && (
            <div
              data-testid="document-stopping-overlay"
              className="absolute inset-0 z-10 flex items-center justify-center rounded bg-[color-mix(in_srgb,var(--bg-card)_60%,transparent)]"
            >
              <Loader2 className="size-4 animate-spin text-text-disabled" />
            </div>
          )}
        </div>
      ) : effectiveRun === RunningStatus.FAIL ? (
        <Button
          variant="danger"
          size="xs"
          className="bg-state-error-5"
          disabled={!canReadDocument}
          onClick={handleShowLog}
          data-testid="document-view-error"
        >
          <TriangleAlert />
          <ParsingErrorLabel record={record}>
            {t('knowledgeDetails.viewError')}
          </ParsingErrorLabel>
        </Button>
      ) : (
        <ParsingCard
          record={record}
          handleShowLog={canReadDocument ? showLog : undefined}
        />
      )}
    </section>
  );
}
