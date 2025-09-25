import { IconFontFill } from '@/components/icon-font';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Modal } from '@/components/ui/modal/modal';
import { cn } from '@/lib/utils';
import { toFixed } from '@/utils/common-util';
import { UseMutateAsyncFunction } from '@tanstack/react-query';
import { t } from 'i18next';
import { lowerFirst } from 'lodash';
import { CirclePause, Trash2, WandSparkles } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { replaceText } from '../../process-log-modal';
import {
  ITraceInfo,
  generateStatus,
  useDatasetGenerate,
  useTraceGenerate,
} from './hook';
export enum GenerateType {
  KnowledgeGraph = 'KnowledgeGraph',
  Raptor = 'Raptor',
}
const MenuItem: React.FC<{
  name: GenerateType;
  data: ITraceInfo;
  pauseGenerate: () => void;
  runGenerate: UseMutateAsyncFunction<
    any,
    Error,
    {
      type: GenerateType;
    },
    unknown
  >;
}> = ({ name, runGenerate, data, pauseGenerate }) => {
  console.log(name, 'pppp', data);
  const iconKeyMap = {
    KnowledgeGraph: 'knowledgegraph',
    Raptor: 'dataflow-01',
  };
  const type = useMemo(() => {
    if (!data) {
      return generateStatus.start;
    }
    if (data.progress >= 1) {
      return generateStatus.completed;
    } else if (!data.progress && data.progress !== 0) {
      return generateStatus.start;
    } else if (data.progress < 0) {
      return generateStatus.failed;
    } else if (data.progress < 1) {
      return generateStatus.running;
    }
  }, [data]);

  const percent =
    type === generateStatus.failed
      ? 100
      : type === generateStatus.running
        ? data.progress * 100
        : 0;

  return (
    <DropdownMenuItem
      className={cn(
        'border cursor-pointer p-2 rounded-md focus:bg-transparent',
        {
          'hover:border-accent-primary hover:bg-[rgba(59,160,92,0.1)]':
            type === generateStatus.start,
          'hover:border-border hover:bg-[rgba(59,160,92,0)]':
            type !== generateStatus.start,
        },
      )}
      onSelect={(e) => {
        e.preventDefault();
      }}
      onClick={(e) => {
        e.stopPropagation();
      }}
    >
      <div
        className="flex items-start gap-2 flex-col w-full"
        onClick={() => {
          if (type === generateStatus.start) {
            runGenerate({ type: name });
          }
        }}
      >
        <div className="flex justify-start text-text-primary items-center gap-2">
          <IconFontFill
            name={iconKeyMap[name]}
            className="text-accent-primary"
          />
          {t(`knowledgeDetails.${lowerFirst(name)}`)}
        </div>
        {type === generateStatus.start && (
          <div className="text-text-secondary text-sm">
            {t(`knowledgeDetails.generate${name}`)}
          </div>
        )}
        {(type === generateStatus.running ||
          type === generateStatus.failed) && (
          <div className="flex justify-between items-center w-full px-2.5 py-1">
            <div
              className={cn(' bg-border-button h-1 rounded-full', {
                'w-[calc(100%-100px)]': type === generateStatus.running,
                'w-[calc(100%-50px)]': type === generateStatus.failed,
              })}
            >
              <div
                className={cn('h-1 rounded-full', {
                  'bg-state-error': type === generateStatus.failed,
                  'bg-accent-primary': type === generateStatus.running,
                })}
                style={{ width: `${toFixed(percent)}%` }}
              ></div>
            </div>
            {type === generateStatus.running && (
              <span>{(toFixed(percent) as string) + '%'}</span>
            )}
            <span
              className="text-state-error"
              onClick={(e) => {
                e.stopPropagation();
                pauseGenerate();
              }}
            >
              {type === generateStatus.failed ? (
                <IconFontFill name="reparse" className="text-accent-primary" />
              ) : (
                <CirclePause />
              )}
            </span>
          </div>
        )}
        <div className="w-full  whitespace-pre-line text-wrap rounded-lg h-fit max-h-[350px] overflow-y-auto scrollbar-auto px-2.5 py-1">
          {replaceText(data?.progress_msg || '')}
        </div>
      </div>
    </DropdownMenuItem>
  );
};

const Generate: React.FC = () => {
  const [open, setOpen] = useState(false);
  const { graphRunData, raptorRunData } = useTraceGenerate({ open });
  const { runGenerate, pauseGenerate } = useDatasetGenerate();
  const handleOpenChange = (isOpen: boolean) => {
    setOpen(isOpen);
    console.log('Dropdown is now', isOpen ? 'open' : 'closed');
  };

  return (
    <div className="generate">
      <DropdownMenu open={open} onOpenChange={handleOpenChange}>
        <DropdownMenuTrigger asChild>
          <Button
            variant={'transparent'}
            onClick={() => {
              handleOpenChange(!open);
            }}
          >
            <WandSparkles className="mr-2" />
            {t('knowledgeDetails.generate')}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent className="w-[380px] p-5 flex flex-col gap-2 ">
          {Object.values(GenerateType).map((name) => {
            const data = (
              name === GenerateType.KnowledgeGraph
                ? graphRunData
                : raptorRunData
            ) as ITraceInfo;
            console.log(
              name,
              'data',
              data,
              !data || (!data.progress && data.progress !== 0),
            );
            return (
              <div key={name}>
                <MenuItem
                  name={name}
                  runGenerate={runGenerate}
                  data={data}
                  pauseGenerate={pauseGenerate}
                />
              </div>
            );
          })}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
};

export default Generate;

export type IGenerateLogProps = {
  id?: string;
  status: 0 | 1;
  message?: string;
  created_at?: string;
  updated_at?: string;
  type?: GenerateType;
  className?: string;
  onDelete?: () => void;
};
export const GenerateLogButton = (props: IGenerateLogProps) => {
  const { t } = useTranslation();
  const {
    id,
    status,
    message,
    created_at,
    updated_at,
    type,
    className,
    onDelete,
  } = props;
  const handleDelete = () => {
    Modal.show({
      visible: true,
      className: '!w-[560px]',
      title:
        t('common.delete') +
        ' ' +
        (type === GenerateType.KnowledgeGraph
          ? t('knowledgeDetails.knowledgeGraph')
          : t('knowledgeDetails.raptor')),
      children: (
        <div
          className="text-sm text-text-secondary"
          dangerouslySetInnerHTML={{
            __html: t('knowledgeConfiguration.deleteGenerateModalContent', {
              type:
                type === GenerateType.KnowledgeGraph
                  ? t('knowledgeDetails.knowledgeGraph')
                  : t('knowledgeDetails.raptor'),
            }),
          }}
        ></div>
      ),
      onVisibleChange: () => {
        Modal.hide();
      },
      footer: (
        <div className="flex justify-end gap-2">
          <Button
            type="button"
            variant={'outline'}
            onClick={() => Modal.hide()}
          >
            {t('dataflowParser.changeStepModalCancelText')}
          </Button>
          <Button
            type="button"
            variant={'secondary'}
            className="!bg-state-error text-text-primary"
            onClick={() => {
              Modal.hide();
            }}
          >
            {t('common.delete')}
          </Button>
        </div>
      ),
    });
  };
  return (
    <div
      className={cn('flex bg-bg-card rounded-md py-1 px-3', props.className)}
    >
      <div className="flex items-center justify-between w-full">
        {status === 1 && (
          <>
            <div>
              {message || t('knowledgeDetails.generatedOn')}
              {created_at}
            </div>
            <Trash2
              size={14}
              className="cursor-pointer"
              onClick={(e) => {
                console.log('delete');
                handleDelete();
                e.stopPropagation();
              }}
            />
          </>
        )}
        {status === 0 && <div>{t('knowledgeDetails.notGenerated')}</div>}
      </div>
    </div>
  );
};
