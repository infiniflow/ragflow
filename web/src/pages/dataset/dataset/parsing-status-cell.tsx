import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from '@/components/ui/hover-card';
import { Progress } from '@/components/ui/progress';
import { Separator } from '@/components/ui/separator';
import { IDocumentInfo } from '@/interfaces/database/document';
import { cn } from '@/lib/utils';
import { CircleX, Play, RefreshCw } from 'lucide-react';
import { PropsWithChildren, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { RunningStatus } from './constant';
import { ParsingCard } from './parsing-card';
import { UseChangeDocumentParserShowType } from './use-change-document-parser';
import { useHandleRunDocumentByIds } from './use-run-document';
import { isParserRunning } from './utils';

const IconMap = {
  [RunningStatus.UNSTART]: <Play />,
  [RunningStatus.RUNNING]: <CircleX />,
  [RunningStatus.CANCEL]: <RefreshCw />,
  [RunningStatus.DONE]: <RefreshCw />,
  [RunningStatus.FAIL]: <RefreshCw />,
};

function MenuItem({
  children,
  onClick,
}: PropsWithChildren & { onClick?(): void }) {
  return (
    <div
      onClick={onClick}
      className={cn(
        'relative flex cursor-default select-none items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none transition-colors focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0',
      )}
    >
      {children}
    </div>
  );
}

export function ParsingStatusCell({
  record,
  showChangeParserModal,
}: { record: IDocumentInfo } & UseChangeDocumentParserShowType) {
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

  return (
    <section className="flex gap-2 items-center ">
      <div>
        <HoverCard>
          <HoverCardTrigger>
            <Button variant={'ghost'} size={'sm'}>
              {parser_id}
            </Button>
          </HoverCardTrigger>
          <HoverCardContent>
            <MenuItem onClick={handleShowChangeParserModal}>
              {t('knowledgeDetails.chunkMethod')}
            </MenuItem>
            <MenuItem>{t('knowledgeDetails.setMetaData')}</MenuItem>
          </HoverCardContent>
        </HoverCard>

        <Separator orientation="vertical" />
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
