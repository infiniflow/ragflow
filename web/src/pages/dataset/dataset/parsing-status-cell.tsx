import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Progress } from '@/components/ui/progress';
import { Separator } from '@/components/ui/separator';
import { IDocumentInfo } from '@/interfaces/database/document';
import { CircleX, Play, RefreshCw } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { RunningStatus } from './constant';
import { ParsingCard } from './parsing-card';
import { UseChangeDocumentParserShowType } from './use-change-document-parser';
import { useHandleRunDocumentByIds } from './use-run-document';
import { UseSaveMetaShowType } from './use-save-meta';
import { isParserRunning } from './utils';

const IconMap = {
  [RunningStatus.UNSTART]: <Play />,
  [RunningStatus.RUNNING]: <CircleX />,
  [RunningStatus.CANCEL]: <RefreshCw />,
  [RunningStatus.DONE]: <RefreshCw />,
  [RunningStatus.FAIL]: <RefreshCw />,
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

  return (
    <section className="flex gap-2 items-center">
      <div className="w-28 flex items-center justify-between">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant={'ghost'} size={'sm'}>
              {parser_id}
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
        <Separator orientation="vertical" className="h-2.5" />
      </div>
      <ConfirmDeleteDialog
        title={t(`knowledgeDetails.redo`, { chunkNum: chunk_num })}
        hidden={isZeroChunk || isRunning}
        onOk={handleOperationIconClick(true)}
        onCancel={handleOperationIconClick(false)}
      >
        <Button
          variant={'ghost'}
          size={'sm'}
          onClick={
            isZeroChunk || isRunning
              ? handleOperationIconClick(false)
              : () => {}
          }
        >
          {operationIcon}
        </Button>
      </ConfirmDeleteDialog>
      {isParserRunning(run) ? (
        <div className="flex items-center gap-1">
          <Progress value={p} className="h-1 flex-1 min-w-10" />
          {p}%
        </div>
      ) : (
        <ParsingCard record={record}></ParsingCard>
      )}
    </section>
  );
}
