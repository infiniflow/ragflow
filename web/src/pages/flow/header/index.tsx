import ChatOverviewModal from '@/components/api-service/chat-overview-modal';
import { useSetModalState, useTranslate } from '@/hooks/common-hooks';
import { useFetchFlow } from '@/hooks/flow-hooks';
import { ArrowLeftOutlined } from '@ant-design/icons';
import { Button, Flex, Space } from 'antd';
import { Link, useParams } from 'umi';
import { useSaveGraph, useSaveGraphBeforeOpeningDebugDrawer } from '../hooks';
import styles from './index.less';

interface IProps {
  showChatDrawer(): void;
}

const FlowHeader = ({ showChatDrawer }: IProps) => {
  const { saveGraph } = useSaveGraph();
  const handleRun = useSaveGraphBeforeOpeningDebugDrawer(showChatDrawer);
  const { data } = useFetchFlow();
  const { t } = useTranslate('flow');
  const {
    visible: overviewVisible,
    hideModal: hideOverviewModal,
    showModal: showOverviewModal,
  } = useSetModalState();
  const { id } = useParams();

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
          <h3>{data.title}</h3>
        </Space>
        <Space size={'large'}>
          <Button onClick={handleRun}>
            <b>{t('run')}</b>
          </Button>
          <Button type="primary" onClick={saveGraph}>
            <b>{t('save')}</b>
          </Button>
          <Button type="primary" onClick={showOverviewModal}>
            <b>{t('publish')}</b>
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
    </>
  );
};

export default FlowHeader;
