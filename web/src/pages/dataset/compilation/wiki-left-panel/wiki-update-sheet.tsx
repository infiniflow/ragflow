import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';

import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { GenerateStatus } from '@/pages/dataset/dataset/generate-button/constants';
import { ITraceInfo } from '@/pages/dataset/dataset/generate-button/hook';
import { useGenerateStatus } from '@/pages/dataset/dataset/generate-button/use-generate-status';

import { ProgressLogPanel } from '../progress-log-panel';

type WikiUpdateSheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  data?: ITraceInfo;
};

export function WikiUpdateSheet({
  open,
  onOpenChange,
  data,
}: WikiUpdateSheetProps) {
  const { t } = useTranslation();
  const { status } = useGenerateStatus(data);

  useEffect(() => {
    if (status === GenerateStatus.completed) {
      onOpenChange(false);
    }
  }, [status, onOpenChange]);

  return (
    <Sheet open={open} onOpenChange={onOpenChange} modal={false}>
      <SheetContent className="flex flex-col">
        <SheetHeader>
          <SheetTitle>
            {t('knowledgeDetails.updateSheetTitle', {
              defaultValue: 'Update Wiki',
            })}
          </SheetTitle>
        </SheetHeader>
        <div className="flex-1 min-h-0 flex flex-col pt-4">
          <ProgressLogPanel
            progressMsg={data?.progress_msg}
            className="flex-1"
          />
        </div>
      </SheetContent>
    </Sheet>
  );
}
