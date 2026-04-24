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
import { CircleX } from 'lucide-react';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { DocumentType, RunningStatus } from './constant';
import { ParsingCard } from './parsing-card';
import { ReparseDialog } from './reparse-dialog';
import { UseChangeDocumentParserShowType } from './use-change-document-parser';
import { useHandleRunDocumentByIds } from './use-run-document';
import { isParserRunning } from './utils';
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
}: {
  record: IDocumentInfo;
  showLog: (record: IDocumentInfo) => void;
} & UseChangeDocumentParserShowType) {
  const { run, progress, chunk_count, id } = record;
  const operationIcon = IconMap[run];
  const p = Number((progress * 100).toFixed(2));
  const {
    handleRunDocumentByIds,
    visible: reparseDialogVisible,
    showModal: showReparseDialogModal,
    hideModal: hideReparseDialogModal,
  } = useHandleRunDocumentByIds(id);
  const isRunning = isParserRunning(run);
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

  const handleShowLog = (record: IDocumentInfo) => {
    showLog(record);
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

          {isParserRunning(run) ? (
            <>
              <Button
                size="auto"
                variant="static"
                onClick={() => handleShowLog(record)}
              >
                <Progress value={p} className="h-1 flex-1 min-w-10" />
                {p}%
              </Button>

              <Button
                variant="ghost"
                size="icon-xs"
                onClick={() => showReparseDialogModal()}
                // onClick={
                //   isZeroChunk || isRunning
                //     ? handleOperationIconClick(false)
                //     : () => {}
                // }
              >
                {operationIcon}
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
          // hidden={false}
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
