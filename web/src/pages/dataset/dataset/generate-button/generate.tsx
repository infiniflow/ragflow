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
import { t } from 'i18next';
import { lowerFirst } from 'lodash';
import { CirclePause, Trash2, WandSparkles } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { generateStatus, useFetchGenerateData } from './hook';
export enum GenerateType {
  KnowledgeGraph = 'KnowledgeGraph',
  Raptor = 'Raptor',
}
const MenuItem: React.FC<{ name: GenerateType }> = ({ name }) => {
  console.log(name, 'pppp');
  const iconKeyMap = {
    KnowledgeGraph: 'knowledgegraph',
    Raptor: 'dataflow-01',
  };
  const {
    data: { percent, type },
    pauseGenerate,
  } = useFetchGenerateData();
  return (
    <div className="flex items-start gap-2 flex-col w-full">
      <div className="flex justify-start text-text-primary items-center gap-2">
        <IconFontFill name={iconKeyMap[name]} className="text-accent-primary" />
        {t(`knowledgeDetails.${lowerFirst(name)}`)}
      </div>
      {type === generateStatus.start && (
        <div className="text-text-secondary text-sm">
          {t(`knowledgeDetails.generate${name}`)}
        </div>
      )}
      {type === generateStatus.running && (
        <div className="flex justify-between items-center w-full">
          <div className="w-[calc(100%-100px)] bg-border-button h-1 rounded-full">
            <div
              className="h-1 bg-accent-primary rounded-full"
              style={{ width: `${toFixed(percent)}%` }}
            ></div>
          </div>
          <span>{toFixed(percent) as string}%</span>
          <span
            className="text-state-error"
            onClick={() => {
              pauseGenerate();
            }}
          >
            <CirclePause />
          </span>
        </div>
      )}
    </div>
  );
};

const Generate: React.FC = () => {
  const [open, setOpen] = useState(false);

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
        <DropdownMenuContent className="w-[380px] p-5  ">
          <DropdownMenuItem
            className="border cursor-pointer p-2 rounded-md hover:border-accent-primary hover:bg-[rgba(59,160,92,0.1)]"
            onSelect={(e) => {
              e.preventDefault();
            }}
            onClick={(e) => {
              e.stopPropagation();
            }}
          >
            <MenuItem name="KnowledgeGraph" />
          </DropdownMenuItem>
          <DropdownMenuItem
            className="border cursor-pointer p-2 rounded-md mt-3 hover:border-accent-primary hover:bg-[rgba(59,160,92,0.1)]"
            onSelect={(e) => {
              e.preventDefault();
            }}
            onClick={(e) => {
              e.stopPropagation();
            }}
          >
            <MenuItem name="Raptor" />
          </DropdownMenuItem>
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
