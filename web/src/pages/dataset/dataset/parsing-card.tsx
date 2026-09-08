import { Button } from '@/components/ui/button';
import { IDocumentInfo } from '@/interfaces/database/document';
import { useTranslation } from 'react-i18next';
import { RunningStatus, RunningStatusMap } from './constant';
import { replaceLogText } from './log-text';

interface IProps {
  record: IDocumentInfo;
  handleShowLog?: (record: IDocumentInfo) => void;
}

function Dot({ color }: { color: string }) {
  return (
    <span
      className={'size-1 inline-block rounded'}
      style={{ backgroundColor: color }}
    ></span>
  );
}

export const PopoverContent = ({ record }: IProps) => {
  const { t } = useTranslation();
  const label = t(`knowledgeDetails.runningStatus${record.run}`);

  const items = [
    {
      key: 'process_begin_at',
      label: t('knowledgeDetails.processBeginAt'),
      children: record.process_begin_at,
    },
    {
      key: 'knowledgeDetails.process_duration',
      label: t('processDuration'),
      children: `${(record.process_duration || 0).toFixed(2)} s`,
    },
    {
      key: 'progress_msg',
      label: t('knowledgeDetails.progressMsg'),
      children: replaceLogText((record.progress_msg || '').trim()),
    },
  ];

  return (
    <section>
      <div className="flex gap-2 items-center pb-2">
        <Dot color={RunningStatusMap[record.run].color}></Dot> {label}
      </div>
      <div className="flex flex-col max-h-[50vh] overflow-auto">
        {items.map((x, idx) => {
          return (
            <div key={x.key} className={idx < 2 ? 'flex gap-2' : ''}>
              <b>{x.label}:</b>
              <div className={'w-full  whitespace-pre-line text-wrap '}>
                {x.children}
              </div>
            </div>
          );
        })}
      </div>
    </section>
  );
};

export function ParsingCard({ record, handleShowLog }: IProps) {
  return (
    <Button
      variant="ghost"
      size="icon-xs"
      onClick={() => handleShowLog?.(record)}
    >
      <Dot color={RunningStatusMap[record.run as RunningStatus].color}></Dot>
    </Button>
  );
}
