import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import { Progress } from '@/components/ui/progress';
import { Separator } from '@/components/ui/separator';
import { IDocumentInfo } from '@/interfaces/database/document';
import { CircleX, Play, RefreshCw } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { RunningStatus } from './constant';
import { ParsingCard } from './parsing-card';
import { useHandleRunDocumentByIds } from './use-run-document';
import { isParserRunning } from './utils';

const IconMap = {
  [RunningStatus.UNSTART]: <Play />,
  [RunningStatus.RUNNING]: <CircleX />,
  [RunningStatus.CANCEL]: <RefreshCw />,
  [RunningStatus.DONE]: <RefreshCw />,
  [RunningStatus.FAIL]: <RefreshCw />,
};

export function ParsingStatusCell({ record }: { record: IDocumentInfo }) {
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

  return (
    <section className="flex gap-2 items-center ">
      <div>
        <Button variant={'ghost'} size={'sm'}>
          {parser_id}
        </Button>
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
