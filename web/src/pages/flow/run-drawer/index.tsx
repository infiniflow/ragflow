import { IModalProps } from '@/interfaces/common';
import { Drawer } from 'antd';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { BeginId } from '../constant';
import DebugContent from '../debug-content';
import { useGetBeginNodeDataQuery } from '../hooks/use-get-begin-query';
import { useSaveGraphBeforeOpeningDebugDrawer } from '../hooks/use-save-graph';
import { BeginQuery } from '../interface';
import useGraphStore from '../store';
import { getDrawerWidth } from '../utils';

const RunDrawer = ({
  hideModal,
  showModal: showChatModal,
}: IModalProps<any>) => {
  const { t } = useTranslation();
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);

  const getBeginNodeDataQuery = useGetBeginNodeDataQuery();
  const query: BeginQuery[] = getBeginNodeDataQuery();

  const { handleRun, loading } = useSaveGraphBeforeOpeningDebugDrawer(
    showChatModal!,
  );

  const handleRunAgent = useCallback(
    (nextValues: Record<string, any>) => {
      const currentNodes = updateNodeForm(BeginId, nextValues, ['query']);
      handleRun(currentNodes);
      hideModal?.();
    },
    [handleRun, hideModal, updateNodeForm],
  );

  const onOk = useCallback(
    async (nextValues: any[]) => {
      handleRunAgent(nextValues);
    },
    [handleRunAgent],
  );

  return (
    <Drawer
      title={t('flow.testRun')}
      placement="right"
      onClose={hideModal}
      open
      getContainer={false}
      width={getDrawerWidth()}
      mask={false}
    >
      <DebugContent
        ok={onOk}
        parameters={query}
        loading={loading}
      ></DebugContent>
    </Drawer>
  );
};

export default RunDrawer;
