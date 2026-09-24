import { IconFontFill } from '@/components/icon-font';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Progress } from '@/components/ui/progress';
import { Separator } from '@/components/ui/separator';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { IDocumentInfo } from '@/interfaces/database/document';
import { cn } from '@/lib/utils';
import { useIsGoBackend } from '@/utils/backend-variant';
import { CircleQuestionMark, CircleX, Clock3, Loader2 } from 'lucide-react';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { DocumentType, RunningStatus } from './constant';
import { ParsingCard } from './parsing-card';
import { ReparseDialog } from './reparse-dialog';
import { UseChangeDocumentParserShowType } from './use-change-document-parser';
import { useHandleRunDocumentByIds } from './use-run-document';
import {
  getDocumentRunningStatus,
  isDocumentProcessing,
  isDocumentStopping,
} from './utils';
const IconMap = {
  [RunningStatus.UNSTART]: (
    <IconFontFill name="play" className="text-accent-primary size-[1em]" />
  ),
  [RunningStatus.RUNNING]: (
    <CircleX color="rgba(var(--state-error))" className="size-[1em]" />
  ),
  [RunningStatus.CANCEL]: (
    <IconFontFill name="reparse" className="text-accent-primary" />
  ),
  [RunningStatus.DONE]: (
    <IconFontFill name="reparse" className="text-accent-primary" />
  ),
  [RunningStatus.FAIL]: (
    <IconFontFill name="reparse" className="text-accent-primary" />
  ),
  [RunningStatus.SCHEDULE]: (
    <IconFontFill name="reparse" className="text-accent-primary" />
  ),
  // QUEUED documents render a dedicated clock button in the processing
  // branch below; this key only keeps the icon map type-exhaustive.
  [RunningStatus.QUEUED]: (
    <IconFontFill name="play" className="text-accent-primary size-[1em]" />
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
  // showSetMetaModal,
}: {
  record: IDocumentInfo;
} & UseChangeDocumentParserShowType) {
  const { t } = useTranslation();
  const { pipeline_id, pipeline_name, chunk_method } = record;

  const handleShowChangeParserModal = useCallback(() => {
    showChangeParserModal(record);
  }, [record, showChangeParserModal]);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <div>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="static" size="auto" className="capitalize">
                {pipeline_id
                  ? pipeline_name || pipeline_id
                  : chunk_method === 'naive'
                    ? 'general'
                    : chunk_method}
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              <p className="capitalize">
                {pipeline_id
                  ? pipeline_name || pipeline_id
                  : chunk_method === 'naive'
                    ? 'general'
                    : chunk_method}
              </p>
            </TooltipContent>
          </Tooltip>
        </div>
      </DropdownMenuTrigger>
      <DropdownMenuContent>
        <DropdownMenuItem onClick={handleShowChangeParserModal}>
          {t('knowledgeDetails.dataPipeline')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function ParsingStatusCell({
  record,
  showLog,
  showChangeParserModal,
}: {
  record: IDocumentInfo;
  showLog: (record: IDocumentInfo) => void;
} & UseChangeDocumentParserShowType) {
  const { t } = useTranslation();
  const { progress, chunk_count, id } = record;
  // Go reports state via ingestion_status (run is gone); Python keeps run.
  // Resolve one effective status for the icon, state attribute and labels.
  const effectiveRun = getDocumentRunningStatus(record);
  const operationIcon = IconMap[effectiveRun];
  const p = Number((progress * 100).toFixed(2));
  const {
    handleRunDocumentByIds,
    loading: isRunLoading,
    visible: reparseDialogVisible,
    showModal: showReparseDialogModal,
    hideModal: hideReparseDialogModal,
  } = useHandleRunDocumentByIds(id, showChangeParserModal);
  const isGo = useIsGoBackend();
  const isRunning = isDocumentProcessing(record);
  const isQueued = effectiveRun === RunningStatus.QUEUED;
  const isStopping = isDocumentStopping(record);
  const isZeroChunk = chunk_count === 0;

  const handleOperationIconClick = (option?: {
    delete: boolean;
    apply_kb: boolean;
  }) => {
    handleRunDocumentByIds(record, isRunning, option);
  };

  // The confirmation only offers real choices when there are existing chunks to
  // drop or auto-metadata to re-apply. Otherwise, and always when cancelling a
  // run, the action fires straight away. Go re-ingests in place server-side, so
  // the dialog is Python-only.
  const needsParseConfirm =
    !isGo &&
    !isRunning &&
    (!isZeroChunk || Boolean(record?.parser_config?.enable_metadata));

  const handleParseClick = () => {
    if (needsParseConfirm) {
      showReparseDialogModal();
      return;
    }
    handleOperationIconClick();
  };

  const showParse = useMemo(() => {
    return record.type !== DocumentType.Virtual;
  }, [record]);

  const handleShowLog = (record: IDocumentInfo) => {
    showLog(record);
  };
  return (
    <section
      className="flex gap-8 items-center"
      data-testid="document-parse-status"
      data-state={
        isQueued
          ? 'queued'
          : isStopping
            ? 'stopping'
            : (ParseStatusStateMap[effectiveRun] ?? 'unknown')
      }
    >
      {showParse && (
        <div className="flex items-center gap-2">
          <Separator orientation="vertical" className="h-[1em]" />

          {isRunning ? (
            <div className="relative">
              {/* While STOPPING the underlying running row stays visible
                  but reads as disabled: dimmed, non-interactive, with both
                  action buttons disabled. The scrim on top carries the
                  spinner. */}
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
                    disabled={isStopping}
                    onClick={() => handleShowLog(record)}
                  >
                    <Clock3 className="size-[1em]" />
                    {t('knowledgeDetails.runningStatusQueued')}
                  </Button>
                ) : (
                  <Button
                    size="auto"
                    variant="static"
                    disabled={isStopping}
                    onClick={() => handleShowLog(record)}
                  >
                    <Progress value={p} className="h-1 flex-1 min-w-10" />
                    <div className="flex items-center gap-1">
                      {p}%
                      <span className="inline-flex items-center">
                        <CircleQuestionMark className="size-[1em]" />
                      </span>
                    </div>
                  </Button>
                )}

                <Button
                  variant="ghost"
                  size="icon-xs"
                  disabled={isStopping}
                  onClick={handleParseClick}
                  data-testid="document-parse-toggle"
                >
                  <CircleX
                    color="rgba(var(--state-error))"
                    className="size-[1em]"
                  />
                </Button>
              </div>

              {/* Go only: STOPPING means a cancel request is in flight but
                  the worker has not reached a terminal state yet. The
                  translucent scrim keeps the dimmed running row visible
                  while the spinner marks the in-flight cancel, until the
                  next poll observes STOPPED/FAILED. */}
              {isStopping && (
                <div
                  data-testid="document-stopping-overlay"
                  className="absolute inset-0 z-10 flex items-center justify-center rounded bg-bg-card/60"
                >
                  <Loader2 className="size-[1em] animate-spin text-text-disabled" />
                </div>
              )}
            </div>
          ) : isGo && isRunLoading ? (
            <Button size="auto" variant="static" disabled>
              <Loader2 className="size-[1em] animate-spin" />
            </Button>
          ) : (
            <>
              <Button
                variant="ghost"
                size="icon-xs"
                onClick={handleParseClick}
                data-testid="document-parse-toggle"
              >
                {operationIcon}
              </Button>

              <ParsingCard record={record} handleShowLog={handleShowLog} />
            </>
          )}
        </div>
      )}
      {reparseDialogVisible && (
        <ReparseDialog
          enable_metadata={record?.parser_config?.enable_metadata}
          alwaysClearChunks={isGo}
          handleOperationIconClick={handleOperationIconClick}
          chunk_num={chunk_count}
          visible={reparseDialogVisible}
          hideModal={hideReparseDialogModal}
        ></ReparseDialog>
      )}
    </section>
  );
}
