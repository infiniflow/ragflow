import EmbedModal from '@/components/api-service/embed-modal';
import { useShowEmbedModal } from '@/components/api-service/hooks';
import { SharedFrom } from '@/constants/chat';
import { useTranslate } from '@/hooks/common-hooks';
import { useFetchFlow } from '@/hooks/flow-hooks';
import { ArrowLeftOutlined } from '@ant-design/icons';
import { Button, Flex, Space } from 'antd';
import { useCallback } from 'react';
import { Link, useParams } from 'umi';
import {
  useGetBeginNodeDataQuery,
  useGetBeginNodeDataQueryIsEmpty,
} from '../hooks/use-get-begin-query';
import {
  useSaveGraph,
  useSaveGraphBeforeOpeningDebugDrawer,
  useWatchAgentChange,
} from '../hooks/use-save-graph';
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
  const { id } = useParams();
  const time = useWatchAgentChange(chatDrawerVisible);
  const getBeginNodeDataQuery = useGetBeginNodeDataQuery();
  const { showEmbedModal, hideEmbedModal, embedVisible, beta } =
    useShowEmbedModal();
  const isBeginNodeDataQueryEmpty = useGetBeginNodeDataQueryIsEmpty();

  const handleShowEmbedModal = useCallback(() => {
    showEmbedModal();
  }, [showEmbedModal]);

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
          <Button
            type="primary"
            onClick={handleShowEmbedModal}
            disabled={!isBeginNodeDataQueryEmpty}
          >
            <b>{t('embedIntoSite', { keyPrefix: 'common' })}</b>
          </Button>
        </Space>
      </Flex>
      {embedVisible && (
        <EmbedModal
          visible={embedVisible}
          hideModal={hideEmbedModal}
          token={id!}
          form={SharedFrom.Agent}
          beta={beta}
          isAgent
        ></EmbedModal>
      )}
    </>
  );
};

export default FlowHeader;
