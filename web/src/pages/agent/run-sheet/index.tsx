import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { IModalProps } from '@/interfaces/common';
import { cn } from '@/lib/utils';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { BeginId } from '../constant';
import DebugContent from '../debug-content';
import { useGetBeginNodeDataInputs } from '../hooks/use-get-begin-query';
import { useSaveGraphBeforeOpeningDebugDrawer } from '../hooks/use-save-graph';
import { BeginQuery } from '../interface';
import useGraphStore from '../store';
import { buildBeginQueryWithObject } from '../utils';

const RunSheet = ({
  hideModal,
  showModal: showChatModal,
}: IModalProps<any>) => {
  const { t } = useTranslation();
  const { updateNodeForm, getNode } = useGraphStore((state) => state);

  const inputs = useGetBeginNodeDataInputs();

  const { handleRun, loading } = useSaveGraphBeforeOpeningDebugDrawer(
    showChatModal!,
  );

  const handleRunAgent = useCallback(
    (nextValues: BeginQuery[]) => {
      const beginNode = getNode(BeginId);
      const inputs: Record<string, BeginQuery> = beginNode?.data.form.inputs;

      const nextInputs = buildBeginQueryWithObject(inputs, nextValues);

      const currentNodes = updateNodeForm(BeginId, nextInputs, ['inputs']);
      handleRun(currentNodes);
      hideModal?.();
    },
    [getNode, handleRun, hideModal, updateNodeForm],
  );

  const onOk = useCallback(
    async (nextValues: any[]) => {
      handleRunAgent(nextValues);
    },
    [handleRunAgent],
  );

  return (
    <Sheet onOpenChange={hideModal} open modal={false}>
      <SheetContent className={cn('top-20 p-2')}>
        <SheetHeader>
          <SheetTitle>{t('flow.testRun')}</SheetTitle>
          <DebugContent
            ok={onOk}
            parameters={inputs}
            loading={loading}
          ></DebugContent>
        </SheetHeader>
      </SheetContent>
    </Sheet>
  );
};

export default RunSheet;
