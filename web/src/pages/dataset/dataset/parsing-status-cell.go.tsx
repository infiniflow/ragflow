import { Button } from '@/components/ui/button';
import { Progress } from '@/components/ui/progress';
import { Separator } from '@/components/ui/separator';
import { IDocumentInfo } from '@/interfaces/database/document';
import { CircleQuestionMark, CircleX, Clock3 } from 'lucide-react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { DocumentType } from './constant';
import {
  isDocumentProcessing,
  isDocumentQueued,
  isDocumentStopping,
} from './document-status.go';
import { ParsingCard } from './parsing-card';
import { ReparseDialog } from './reparse-dialog';
import { useHandleRunDocumentByIds } from './use-run-document';
import {
  ParseStatusStateMap,
  StatusOperationIcon,
} from './parsing-status-icons';

export function ParsingStatusCellGo({
  record,
  showLog,
}: {
  record: IDocumentInfo;
  showLog: (record: IDocumentInfo) => void;
}) {
  const { t } = useTranslation();
  const { run, progress, chunk_count, id } = record;
  const operationIcon = StatusOperationIcon[run];
  const p = Number((progress * 100).toFixed(2));
  const {
    handleRunDocumentByIds,
    visible: reparseDialogVisible,
    showModal: showReparseDialogModal,
    hideModal: hideReparseDialogModal,
  } = useHandleRunDocumentByIds(id);
  const isRunning = isDocumentProcessing(record);
  const isQueued = isDocumentQueued(record);
  const isStopping = isDocumentStopping(record);
  const isZeroChunk = chunk_count === 0;

  const handleOperationIconClick = (option?: {
    delete: boolean;
    apply_kb: boolean;
  }) => {
    handleRunDocumentByIds(record.id, isRunning, option);
  };

  const showParse = useMemo(() => {
    return record.type !== DocumentType.Virtual;
  }, [record]);

  const handleShowLog = (current: IDocumentInfo) => {
    showLog(current);
  };
  return (
    <section
      className="flex gap-8 items-center"
      data-testid="document-parse-status"
      data-state={isQueued ? 'queued' : (ParseStatusStateMap[run] ?? 'unknown')}
    >
      {showParse && (
        <div className="flex items-center gap-2">
          <Separator orientation="vertical" className="h-[1em]" />

          {isRunning ? (
            <>
              {isQueued ? (
                <Button
                  size="auto"
                  variant="static"
                  onClick={() => handleShowLog(record)}
                >
                  <Clock3 className="size-[1em]" />
                  {t('knowledgeDetails.runningStatusQueued')}
                </Button>
              ) : (
                <Button
                  size="auto"
                  variant="static"
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
                onClick={() => showReparseDialogModal()}
              >
                <CircleX
                  color="rgba(var(--state-error))"
                  className="size-[1em]"
                />
              </Button>
            </>
          ) : (
            <>
              <Button
                variant="ghost"
                size="icon-xs"
                onClick={() => {
                  showReparseDialogModal();
                }}
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
          hidden={
            (isZeroChunk && !record?.parser_config?.enable_metadata) ||
            isRunning
          }
          enable_metadata={record?.parser_config?.enable_metadata}
          handleOperationIconClick={handleOperationIconClick}
          chunk_num={chunk_count}
          visible={reparseDialogVisible}
          hideModal={hideReparseDialogModal}
        ></ReparseDialog>
      )}
    </section>
  );
}
