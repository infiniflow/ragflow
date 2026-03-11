import { RAGFlowAvatar } from '@/components/ragflow-avatar';
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
import { IFlow } from '@/interfaces/database/agent';
import { formatDate } from '@/utils/date';
import { BookPlus } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

interface PublishConfirmDialogProps {
  agentDetail: IFlow;
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

  const lastPublished = useMemo(() => {
    if (agentDetail?.last_publish_time) {
      return formatDate(agentDetail.last_publish_time);
    }
    return '';
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
            <div className="flex gap-2.5 bg-bg-input px-2.5 py-4 rounded items-center">
              <RAGFlowAvatar
                avatar={agentDetail.avatar}
                name={agentDetail.title}
              />
              <span className="text-text-primary text-lg">
                {agentDetail.title}
              </span>
            </div>
            <div className="flex flex-col gap-2">
              {lastPublished && (
                <div className="flex items-center text-sm text-text-secondary gap-2">
                  <span>{t('flow.lastPublished')}:</span>
                  <span>{lastPublished}</span>
                </div>
              )}
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
