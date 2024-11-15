import ChatOverviewModal from '@/components/api-service/chat-overview-modal';
import { useSetModalState, useTranslate } from '@/hooks/common-hooks';
import { useFetchFlow } from '@/hooks/flow-hooks';
import { ArrowLeftOutlined } from '@ant-design/icons';
import { Button, Flex, Space } from 'antd';
import { useCallback } from 'react';
import { Link, useParams } from 'umi';
import FlowIdModal from '../flow-id-modal';
import {
  useGetBeginNodeDataQuery,
  useSaveGraph,
  useSaveGraphBeforeOpeningDebugDrawer,
  useWatchAgentChange,
} from '../hooks';
import { BeginQuery } from '../interface';
import styles from './index.less';

interface IProps {
  showChatDrawer(): void;
  chatDrawerVisible: boolean;
}

const FlowHeader = ({ showChatDrawer, chatDrawerVisible }: IProps) => {
  const { saveGraph } = useSaveGraph();
  const { handleRun } = useSaveGraphBeforeOpeningDebugDrawer(showChatDrawer);
  const { data } = useFetchFlow();
  const { t } = useTranslate('flow');
  const {
    visible: overviewVisible,
    hideModal: hideOverviewModal,
    // showModal: showOverviewModal,
  } = useSetModalState();
  const { visible, hideModal, showModal } = useSetModalState();
  const { id } = useParams();
  const time = useWatchAgentChange(chatDrawerVisible);
  const getBeginNodeDataQuery = useGetBeginNodeDataQuery();

  const handleRunAgent = useCallback(() => {
    const query: BeginQuery[] = getBeginNodeDataQuery();
    if (query.length > 0) {
      showChatDrawer();
    } else {
      handleRun();
    }
  }, [getBeginNodeDataQuery, handleRun, showChatDrawer]);

  return (
    <>
      <Flex
        align="center"
        justify={'space-between'}
        gap={'large'}
        className={styles.flowHeader}
      >
        <Space size={'large'}>
          <Link to={`/flow`}>
            <ArrowLeftOutlined />
          </Link>
          <div className="flex flex-col">
            <span className="font-semibold text-[18px]">{data.title}</span>
            <span className="font-normal text-sm">
              {t('autosaved')} {time}
            </span>
          </div>
        </Space>
        <Space size={'large'}>
          <Button onClick={handleRunAgent}>
            <b>{t('run')}</b>
          </Button>
          <Button type="primary" onClick={() => saveGraph()}>
            <b>{t('save')}</b>
          </Button>
          {/* <Button type="primary" onClick={showOverviewModal} disabled>
            <b>{t('publish')}</b>
          </Button> */}
          <Button type="primary" onClick={showModal}>
            <b>Agent ID</b>
          </Button>
        </Space>
      </Flex>
      {overviewVisible && (
        <ChatOverviewModal
          visible={overviewVisible}
          hideModal={hideOverviewModal}
          id={id!}
          idKey="canvasId"
        ></ChatOverviewModal>
      )}
      {visible && <FlowIdModal hideModal={hideModal}></FlowIdModal>}
    </>
  );
};

export default FlowHeader;
