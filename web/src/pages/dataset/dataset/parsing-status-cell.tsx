import { IconFontFill } from '@/components/icon-font';
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
    <IconFontFill name="play" className="text-accent-primary" />
  ),
  [RunningStatus.RUNNING]: (
    <CircleX size={14} color="rgba(var(--state-error))" />
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
};

export function ParsingStatusCell({
  record,
  showChangeParserModal,
  // showSetMetaModal,
  showLog,
}: {
  record: IDocumentInfo;
  showLog: (record: IDocumentInfo) => void;
} & UseChangeDocumentParserShowType) {
  const { t } = useTranslation();
  const {
    run,
    parser_id,
    pipeline_id,
    pipeline_name,
    progress,
    chunk_num,
    id,
  } = record;
  const operationIcon = IconMap[run];
  const p = Number((progress * 100).toFixed(2));
  const {
    handleRunDocumentByIds,
    visible: reparseDialogVisible,
    showModal: showReparseDialogModal,
    hideModal: hideReparseDialogModal,
  } = useHandleRunDocumentByIds(id);
  const isRunning = isParserRunning(run);
  const isZeroChunk = chunk_num === 0;

  const handleOperationIconClick = (option?: {
    delete: boolean;
    apply_kb: boolean;
  }) => {
    handleRunDocumentByIds(record.id, isRunning, option);
  };

  const handleShowChangeParserModal = useCallback(() => {
    showChangeParserModal(record);
  }, [record, showChangeParserModal]);

  const showParse = useMemo(() => {
    return record.type !== DocumentType.Virtual;
  }, [record]);

  const handleShowLog = (record: IDocumentInfo) => {
    showLog(record);
  };
  return (
    <section className="flex gap-8 items-center">
      <div className="text-ellipsis w-[100px] flex items-center justify-between">
        <DropdownMenu>
          <DropdownMenuTrigger>
            <Tooltip>
              <TooltipTrigger asChild>
                <div className="border-none truncate max-w-32 cursor-pointer px-2 py-1 rounded-sm hover:bg-bg-card">
                  {pipeline_id
                    ? pipeline_name || pipeline_id
                    : parser_id === 'naive'
                      ? 'general'
                      : parser_id}
                </div>
              </TooltipTrigger>
              <TooltipContent>
                <p>
                  {pipeline_id
                    ? pipeline_name || pipeline_id
                    : parser_id === 'naive'
                      ? 'general'
                      : parser_id}
                </p>
              </TooltipContent>
            </Tooltip>
          </DropdownMenuTrigger>
          <DropdownMenuContent>
            <DropdownMenuItem onClick={handleShowChangeParserModal}>
              {t('knowledgeDetails.dataPipeline')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {showParse && (
        <div className="flex items-center gap-3">
          <Separator orientation="vertical" className="h-2.5" />
          {!isParserRunning(run) && (
            // <ReparseDialog
            //   hidden={isZeroChunk || isRunning}
            //   handleOperationIconClick={handleOperationIconClick}
            //   chunk_num={chunk_num}
            // >
            <div
              className="cursor-pointer flex items-center gap-3"
              onClick={() => {
                showReparseDialogModal();
              }}
            >
              {!isParserRunning(run) && operationIcon}
            </div>
            // {/* </ReparseDialog> */}
          )}
          {isParserRunning(run) ? (
            <>
              <div
                className="flex items-center gap-1 cursor-pointer"
                onClick={() => handleShowLog(record)}
              >
                <Progress value={p} className="h-1 flex-1 min-w-10" />
                {p}%
              </div>
              <div
                className="cursor-pointer flex items-center gap-3"
                onClick={() => {
                  showReparseDialogModal();
                }}
                // onClick={
                //   isZeroChunk || isRunning
                //     ? handleOperationIconClick(false)
                //     : () => {}
                // }
              >
                {operationIcon}
              </div>
            </>
          ) : (
            <ParsingCard
              record={record}
              handleShowLog={handleShowLog}
            ></ParsingCard>
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
          chunk_num={chunk_num}
          visible={reparseDialogVisible}
          hideModal={hideReparseDialogModal}
        ></ReparseDialog>
      )}
    </section>
  );
}
