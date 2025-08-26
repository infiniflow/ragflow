import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from '@/components/ui/hover-card';
import { Progress } from '@/components/ui/progress';
import { Separator } from '@/components/ui/separator';
import { IDocumentInfo } from '@/interfaces/database/document';
import { CircleX, RefreshCw } from 'lucide-react';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { DocumentType, RunningStatus } from './constant';
import { ParsingCard, PopoverContent } from './parsing-card';
import { UseChangeDocumentParserShowType } from './use-change-document-parser';
import { useHandleRunDocumentByIds } from './use-run-document';
import { UseSaveMetaShowType } from './use-save-meta';
import { isParserRunning } from './utils';
const IconMap = {
  [RunningStatus.UNSTART]: (
    <div className="w-0 h-0 border-l-[10px] border-l-accent-primary border-t-8 border-r-4 border-b-8 border-transparent"></div>
  ),
  [RunningStatus.RUNNING]: <CircleX size={14} color="var(--state-error)" />,
  [RunningStatus.CANCEL]: <RefreshCw size={14} color="var(--accent-primary)" />,
  [RunningStatus.DONE]: <RefreshCw size={14} color="var(--accent-primary)" />,
  [RunningStatus.FAIL]: <RefreshCw size={14} color="var(--accent-primary)" />,
};

export function ParsingStatusCell({
  record,
  showChangeParserModal,
  showSetMetaModal,
}: { record: IDocumentInfo } & UseChangeDocumentParserShowType &
  UseSaveMetaShowType) {
  const { t } = useTranslation();
  const { run, parser_id, progress, chunk_num, id } = record;
  const operationIcon = IconMap[run];
  const p = Number((progress * 100).toFixed(2));
  const { handleRunDocumentByIds } = useHandleRunDocumentByIds(id);
  const isRunning = isParserRunning(run);
  const isZeroChunk = chunk_num === 0;

  const handleOperationIconClick =
    (shouldDelete: boolean = false) =>
    () => {
      handleRunDocumentByIds(record.id, isRunning, shouldDelete);
    };

  const handleShowChangeParserModal = useCallback(() => {
    showChangeParserModal(record);
  }, [record, showChangeParserModal]);

  const handleShowSetMetaModal = useCallback(() => {
    showSetMetaModal(record);
  }, [record, showSetMetaModal]);

  const showParse = useMemo(() => {
    return record.type !== DocumentType.Virtual;
  }, [record]);

  return (
    <section className="flex gap-8 items-center">
      <div className="w-fit flex items-center justify-between">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant={'transparent'} className="border-none" size={'sm'}>
              {parser_id === 'naive' ? 'general' : parser_id}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent>
            <DropdownMenuItem onClick={handleShowChangeParserModal}>
              {t('knowledgeDetails.chunkMethod')}
            </DropdownMenuItem>
            <DropdownMenuItem onClick={handleShowSetMetaModal}>
              {t('knowledgeDetails.setMetaData')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      {showParse && (
        <>
          <ConfirmDeleteDialog
            title={t(`knowledgeDetails.redo`, { chunkNum: chunk_num })}
            hidden={isZeroChunk || isRunning}
            onOk={handleOperationIconClick(true)}
            onCancel={handleOperationIconClick(false)}
          >
            <div
              className="cursor-pointer flex items-center gap-3"
              onClick={
                isZeroChunk || isRunning
                  ? handleOperationIconClick(false)
                  : () => {}
              }
            >
              <Separator orientation="vertical" className="h-2.5" />
              {operationIcon}
            </div>
          </ConfirmDeleteDialog>
          {isParserRunning(run) ? (
            <HoverCard>
              <HoverCardTrigger asChild>
                <div className="flex items-center gap-1">
                  <Progress value={p} className="h-1 flex-1 min-w-10" />
                  {p}%
                </div>
              </HoverCardTrigger>
              <HoverCardContent className="w-[40vw]">
                <PopoverContent record={record}></PopoverContent>
              </HoverCardContent>
            </HoverCard>
          ) : (
            <ParsingCard record={record}></ParsingCard>
          )}
        </>
      )}
    </section>
  );
}
