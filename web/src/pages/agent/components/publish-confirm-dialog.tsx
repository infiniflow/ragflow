import { Button, ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import { Operator } from '@/pages/agent/constant';
import useGraphStore from '@/pages/agent/store';
import { formatDate } from '@/utils/date';
import { BookPlus } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

interface PublishConfirmDialogProps {
  agentDetail: { title: string; update_time?: number };
  loading: boolean;
  onPublish: () => void;
}

export function PublishConfirmDialog({
  agentDetail,
  loading,
  onPublish,
}: PublishConfirmDialogProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const nodes = useGraphStore((state) => state.nodes);

  const linkedDatasets = useMemo(() => {
    const datasets: string[] = [];
    nodes.forEach((node) => {
      if (node.data.label === Operator.Retrieval) {
        const kbIds = node.data.form?.kb_ids || [];
        datasets.push(...kbIds);
      }
    });
    return [...new Set(datasets)];
  }, [nodes]);

  const lastPublished = useMemo(() => {
    if (agentDetail?.update_time) {
      return formatDate(agentDetail.update_time);
    }
    return '-';
  }, [agentDetail?.update_time]);

  const handleConfirmPublish = () => {
    onPublish();
    setOpen(false);
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <ButtonLoading variant={'secondary'} loading={loading}>
          <BookPlus /> {t('flow.release')}
        </ButtonLoading>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('flow.confirmPublish')}</DialogTitle>
        </DialogHeader>
        <DialogDescription>
          <div className="space-y-4">
            <div className="flex flex-col gap-1">
              <span className="text-sm font-medium text-text-primary">
                {agentDetail.title}
              </span>
            </div>
            <div className="flex flex-col gap-2">
              <div className="flex items-center justify-between text-sm">
                <span className="text-text-secondary">
                  {t('flow.linkedDataset')}
                </span>
                <span className="text-text-primary">
                  {linkedDatasets.length > 0
                    ? linkedDatasets.join(', ')
                    : t('common.none')}
                </span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-text-secondary">
                  {t('flow.lastPublished')}
                </span>
                <span className="text-text-primary">{lastPublished}</span>
              </div>
            </div>
          </div>
        </DialogDescription>
        <DialogFooter className="gap-2 mt-4">
          <Button variant="outline" onClick={() => setOpen(false)}>
            {t('common.cancel')}
          </Button>
          <Button onClick={handleConfirmPublish}>{t('common.confirm')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
