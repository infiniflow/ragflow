import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { ButtonLoading } from '@/components/ui/button';
import { IFlow } from '@/interfaces/database/agent';
import { formatDate } from '@/utils/date';
import { BookPlus } from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useIsPipeline } from '../hooks/use-is-pipeline';
import { AliasFormDialog } from './alias-form-dialog';

interface PublishConfirmDialogProps {
  agentDetail: IFlow;
  loading: boolean;
  onPublish: (alias?: string) => void;
}

export function PublishConfirmDialog({
  agentDetail,
  loading,
  onPublish,
}: PublishConfirmDialogProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const isPipeline = useIsPipeline();

  const lastPublished = useMemo(() => {
    if (agentDetail?.last_publish_time) {
      return formatDate(agentDetail.last_publish_time);
    }
    return '';
  }, [agentDetail.last_publish_time]);

  const handleTriggerClick = useCallback(() => {
    setOpen(true);
  }, []);

  return (
    <>
      <ButtonLoading
        variant={'secondary'}
        loading={loading}
        onClick={handleTriggerClick}
      >
        <BookPlus /> {t('flow.release')}
      </ButtonLoading>
      <AliasFormDialog
        open={open}
        onOpenChange={setOpen}
        title={t('flow.confirmPublish')}
        description={t(
          `flow.${isPipeline ? 'publishIngestionPipeline' : 'publishAgent'}`,
        )}
        onSubmit={onPublish}
      >
        <section className="bg-bg-input px-2.5 py-4 rounded border border-border-default">
          <div className="flex gap-2.5 items-center">
            <RAGFlowAvatar
              avatar={agentDetail.avatar}
              name={agentDetail.title}
              className="size-8"
            />
            <span className="text-text-primary text-lg">
              {agentDetail.title}
            </span>
          </div>
        </section>

        {lastPublished && (
          <div className="flex items-center text-sm text-text-secondary gap-2">
            <span>{t('flow.lastPublished')}:</span>
            <span>{lastPublished}</span>
          </div>
        )}
      </AliasFormDialog>
    </>
  );
}
