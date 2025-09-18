import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { IModalProps } from '@/interfaces/common';
import { cn } from '@/lib/utils';
import { useTranslation } from 'react-i18next';
import { useRunDataflow } from '../hooks/use-run-dataflow';
import { UploaderForm } from './uploader';

const RunSheet = ({ hideModal, showModal: showLogSheet }: IModalProps<any>) => {
  const { t } = useTranslation();

  const { run, loading } = useRunDataflow(() => {});

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

export default RunSheet;
