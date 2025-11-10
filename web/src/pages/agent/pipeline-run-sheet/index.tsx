import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { IModalProps } from '@/interfaces/common';
import { cn } from '@/lib/utils';
import { useTranslation } from 'react-i18next';
import { RunDataflowType } from '../hooks/use-run-dataflow';
import { UploaderForm } from './uploader';

type RunSheetProps = IModalProps<any> &
  Pick<RunDataflowType, 'run' | 'loading'>;

const PipelineRunSheet = ({ hideModal, run, loading }: RunSheetProps) => {
  const { t } = useTranslation();

  return (
    <Sheet onOpenChange={hideModal} open modal={false}>
      <SheetContent className={cn('top-20 p-2')}>
        <SheetHeader>
          <SheetTitle>{t('flow.testRun')}</SheetTitle>
          <UploaderForm ok={run} loading={loading}></UploaderForm>
        </SheetHeader>
      </SheetContent>
    </Sheet>
  );
};

export default PipelineRunSheet;
