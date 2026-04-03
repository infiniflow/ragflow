import { Button } from '@/components/ui/button';
import { IDocumentInfo } from '@/interfaces/database/document';
import { useTranslation } from 'react-i18next';
import reactStringReplace from 'react-string-replace';
import { RunningStatus, RunningStatusMap } from './constant';

interface IProps {
  record: IDocumentInfo;
  handleShowLog?: (record: IDocumentInfo) => void;
}

function Dot({ run }: { run: RunningStatus }) {
  const runningStatus = RunningStatusMap[run];
  return (
    <span
      className={'size-1 inline-block rounded'}
      style={{ backgroundColor: runningStatus.color }}
    ></span>
  );
}

export const PopoverContent = ({ record }: IProps) => {
  const { t } = useTranslation();
  const label = t(`knowledgeDetails.runningStatus${record.run}`);

  const replaceText = (text: string) => {
    // Remove duplicate \n
    const nextText = text.replace(/(\n)\1+/g, '$1');

    const replacedText = reactStringReplace(
      nextText,
      /(\[ERROR\].+\s)/g,
      (match, i) => {
        return (
          <span key={i} className={'text-red-600'}>
            {match}
          </span>
        );
      },
    );

    return replacedText;
  };

  const items = [
    {
      key: 'process_begin_at',
      label: t('knowledgeDetails.processBeginAt'),
      children: record.process_begin_at,
    },
    {
      key: 'knowledgeDetails.process_duration',
      label: t('processDuration'),
      children: `${record.process_duration.toFixed(2)} s`,
    },
    {
      key: 'progress_msg',
      label: t('knowledgeDetails.progressMsg'),
      children: replaceText(record.progress_msg.trim()),
    },
  ];

  return (
    <section>
      <div className="flex gap-2 items-center pb-2">
        <Dot run={record.run}></Dot> {label}
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
      variant={'transparent'}
      className="border-none"
      size={'sm'}
      onClick={() => handleShowLog?.(record)}
    >
      <Dot run={record.run}></Dot>
    </Button>
  );
}
