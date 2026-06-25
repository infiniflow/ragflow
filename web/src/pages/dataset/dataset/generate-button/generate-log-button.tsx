import { Button } from '@/components/ui/button';
import { Modal } from '@/components/ui/modal/modal';
import { cn } from '@/lib/utils';
import { formatDate } from '@/utils/date';
import { Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { GenerateType, GenerateTypeMap } from './constants';
import { useUnBindTask } from './hook';

export type IGenerateLogButtonProps = {
  finish_at: string;
  task_id: string;
};

export type IGenerateLogProps = IGenerateLogButtonProps & {
  id?: string;
  status: 0 | 1;
  message?: string;
  created_at?: string;
  updated_at?: string;
  type?: GenerateType;
  className?: string;
  onDelete?: () => void;
};

export function GenerateLogButton(props: IGenerateLogProps) {
  const { t } = useTranslation();
  const { message, finish_at, type, onDelete } = props;

  const { handleUnbindTask } = useUnBindTask();

  const handleDeleteFunc = async () => {
    const data = await handleUnbindTask({
      type: GenerateTypeMap[type as GenerateType],
    });
    Modal.destroy();
    if (data.code === 0) {
      onDelete?.();
    }
  };

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
        Modal.destroy();
      },
      footer: (
        <div className="flex justify-end gap-2">
          <Button
            type="button"
            variant={'outline'}
            onClick={() => Modal.destroy()}
          >
            {t('dataflowParser.changeStepModalCancelText')}
          </Button>
          <Button
            type="button"
            variant={'secondary'}
            className="!bg-state-error text-text-primary"
            onClick={() => {
              handleDeleteFunc();
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
        {finish_at && (
          <>
            <div>
              {message || t('knowledgeDetails.generatedOn')}
              {formatDate(finish_at)}
            </div>
            <Trash2
              size={14}
              className="cursor-pointer"
              onClick={(e) => {
                handleDelete();
                e.stopPropagation();
              }}
            />
          </>
        )}
        {!finish_at && <div>{t('knowledgeDetails.notGenerated')}</div>}
      </div>
    </div>
  );
}

export default GenerateLogButton;
