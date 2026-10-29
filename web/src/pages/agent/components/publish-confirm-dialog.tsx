import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { ButtonLoading } from '@/components/ui/button';
import { IFlow } from '@/interfaces/database/agent';
import { IDataset } from '@/interfaces/database/dataset';
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

function AssociatedDataset({
  associatedDatasets,
}: {
  associatedDatasets: Pick<IDataset, 'id' | 'name' | 'avatar'>[];
}) {
  const { t } = useTranslation();

  return (
    <div className="space-y-2 pl-10 pt-3">
      <div className="text-sm font-medium text-text-secondary">
        {t('flow.linkedDataset')}
      </div>
      {associatedDatasets.length > 0 ? (
        <div className="space-y-2 max-h-32 overflow-y-auto">
          {associatedDatasets.map((dataset) => (
            <div
              key={dataset.id}
              className="flex items-center gap-2 px-2 py-2 bg-bg-card rounded text-sm text-text-primary"
            >
              <RAGFlowAvatar
                avatar={dataset.avatar}
                name={dataset.name}
                className="size-4 text-xs"
              />
              <span className="truncate text-text-secondary">
                {dataset.name}
              </span>
            </div>
          ))}
        </div>
      ) : (
        <div className="text-sm text-text-disabled">{t('common.noData')}</div>
      )}
    </div>
  );
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

  // Get datasets associated with this pipeline from API response
  const associatedDatasets = useMemo(() => {
    return agentDetail?.datasets || [];
  }, [agentDetail?.datasets]);

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

          {isPipeline && (
            <AssociatedDataset
              associatedDatasets={associatedDatasets}
            ></AssociatedDataset>
          )}
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
