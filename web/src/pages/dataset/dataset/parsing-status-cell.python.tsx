import { IconFontFill } from '@/components/icon-font';
import { Button } from '@/components/ui/button';
import { Separator } from '@/components/ui/separator';
import { IDocumentInfo } from '@/interfaces/database/document';
import { CircleX } from 'lucide-react';
import { useMemo } from 'react';
import { DocumentType, RunningStatus } from './constant';
import { isDocumentProcessing } from './document-status.python';
import { ParsingCard } from './parsing-card';
import { ReparseDialog } from './reparse-dialog';
import { UseChangeDocumentParserShowType } from './use-change-document-parser';
import { useHandleRunDocumentByIds } from './use-run-document';

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
};

const ParseStatusStateMap = {
  [RunningStatus.UNSTART]: 'unstart',
  [RunningStatus.RUNNING]: 'running',
  [RunningStatus.CANCEL]: 'cancel',
  [RunningStatus.DONE]: 'success',
  [RunningStatus.FAIL]: 'fail',
  [RunningStatus.SCHEDULE]: 'running',
} as const;

export function ParsingStatusCellPython({
  record,
  showLog,
}: {
  record: IDocumentInfo;
  showLog: (record: IDocumentInfo) => void;
} & UseChangeDocumentParserShowType) {
  const { run, chunk_count, id } = record;
  const operationIcon = IconMap[run];
  const {
    handleRunDocumentByIds,
    visible: reparseDialogVisible,
    showModal: showReparseDialogModal,
    hideModal: hideReparseDialogModal,
  } = useHandleRunDocumentByIds(id);
  const isRunning = isDocumentProcessing(record);
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
      data-state={ParseStatusStateMap[run] ?? 'unknown'}
    >
      {showParse && (
        <div className="flex items-center gap-2">
          <Separator orientation="vertical" className="h-[1em]" />

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
