import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { IModalProps } from '@/interfaces/common';
import { cn } from '@/lib/utils';
import { useTranslation } from 'react-i18next';
import { useMemo, useState } from 'react';
import { Operator } from '../constant';
import { RunDataflowType } from '../hooks/use-run-dataflow';
import useGraphStore from '../store';
import { UploaderForm } from './uploader';

type RunSheetProps = IModalProps<any> &
  Pick<RunDataflowType, 'run' | 'loading'>;

const PipelineRunSheet = ({ hideModal, run, loading }: RunSheetProps) => {
  const { t } = useTranslation();
  const nodes = useGraphStore((state) => state.nodes);
  const [targetComponentId, setTargetComponentId] = useState('');
  const titleChunkerNodes = useMemo(
    () => nodes.filter((node) => node.data.label === Operator.TitleChunker),
    [nodes],
  );

  const handleRun = (values: Parameters<RunDataflowType['run']>[0]) => {
    run(values, targetComponentId || undefined);
  };

  return (
    <Sheet onOpenChange={hideModal} open modal={false}>
      <SheetContent className={cn('top-20 p-2')}>
        <SheetHeader>
          <SheetTitle>{t('flow.testRun')}</SheetTitle>
          {titleChunkerNodes.length > 0 && (
            <label className="space-y-2 text-sm">
              <span className="text-text-secondary">Preview target</span>
              <select
                className="h-9 w-full rounded-md border bg-background px-3"
                onChange={(event) => setTargetComponentId(event.target.value)}
                value={targetComponentId}
              >
                <option value="">Run full pipeline</option>
                {titleChunkerNodes.map((node) => (
                  <option key={node.id} value={node.id}>
                    {node.data.name || node.id}
                  </option>
                ))}
              </select>
            </label>
          )}
          <UploaderForm ok={handleRun} loading={loading}></UploaderForm>
        </SheetHeader>
      </SheetContent>
    </Sheet>
  );
};

export default PipelineRunSheet;
