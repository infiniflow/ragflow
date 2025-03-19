import EmbedModal from '@/components/api-service/embed-modal';
import { useShowEmbedModal } from '@/components/api-service/hooks';
import { SharedFrom } from '@/constants/chat';
import { useTranslate } from '@/hooks/common-hooks';
import { useFetchFlow } from '@/hooks/flow-hooks';
import { useFetchUserInfo } from '@/hooks/user-setting-hooks';
import { ArrowLeftOutlined } from '@ant-design/icons';
import { Badge, Button, Flex, Space } from 'antd';
import classNames from 'classnames';
import { useCallback } from 'react';
import { Link, useParams } from 'umi';
import { FlowSettingModal, useFlowSettingModal } from '../flow-setting';
import {
  useGetBeginNodeDataQuery,
  useGetBeginNodeDataQueryIsSafe,
} from '../hooks/use-get-begin-query';
import {
  useSaveGraph,
  useSaveGraphBeforeOpeningDebugDrawer,
  useWatchAgentChange,
} from '../hooks/use-save-graph';
import { BeginQuery } from '../interface';

import {
  HistoryVersionModal,
  useHistoryVersionModal,
} from '../history-version-modal';
import styles from './index.less';

interface IProps {
  showChatDrawer(): void;
  chatDrawerVisible: boolean;
}

const FlowHeader = ({ showChatDrawer, chatDrawerVisible }: IProps) => {
  const { saveGraph } = useSaveGraph();
  const { handleRun } = useSaveGraphBeforeOpeningDebugDrawer(showChatDrawer);
  const { data: userInfo } = useFetchUserInfo();

  const { data } = useFetchFlow();
  const { t } = useTranslate('flow');
  const { id } = useParams();
  const time = useWatchAgentChange(chatDrawerVisible);
  const getBeginNodeDataQuery = useGetBeginNodeDataQuery();
  const { showEmbedModal, hideEmbedModal, embedVisible, beta } =
    useShowEmbedModal();
  const { setVisibleSettingMModal, visibleSettingModal } =
    useFlowSettingModal();
  const isBeginNodeDataQuerySafe = useGetBeginNodeDataQueryIsSafe();
  const { setVisibleHistoryVersionModal, visibleHistoryVersionModal } =
    useHistoryVersionModal();
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

  const showSetting = useCallback(() => {
    setVisibleSettingMModal(true);
  }, [setVisibleSettingMModal]);

  const showListVersion = useCallback(() => {
    setVisibleHistoryVersionModal(true);
  }, [setVisibleHistoryVersionModal]);
  return (
    <>
      <Flex
        align="center"
        justify={'space-between'}
        gap={'large'}
        className={styles.flowHeader}
      >
        <Badge.Ribbon
          text={data?.nickname}
          style={{ marginRight: -data?.nickname?.length * 5 }}
          color={userInfo?.nickname === data?.nickname ? '#1677ff' : 'pink'}
          className={classNames(styles.ribbon, {
            [styles.hideRibbon]: data.permission !== 'team',
          })}
        >
          <Space className={styles.headerTitle} size={'large'}>
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
        </Badge.Ribbon>
        <Space size={'large'}>
          <Button
            disabled={userInfo.nickname !== data.nickname}
            onClick={handleRunAgent}
          >
            <b>{t('run')}</b>
          </Button>
          <Button
            disabled={userInfo.nickname !== data.nickname}
            type="primary"
            onClick={() => saveGraph()}
          >
            <b>{t('save')}</b>
          </Button>
          <Button
            type="primary"
            onClick={handleShowEmbedModal}
            disabled={
              !isBeginNodeDataQuerySafe || userInfo.nickname !== data.nickname
            }
          >
            <b>{t('embedIntoSite', { keyPrefix: 'common' })}</b>
          </Button>
          <Button
            disabled={userInfo.nickname !== data.nickname}
            type="primary"
            onClick={showSetting}
          >
            <b>{t('setting')}</b>
          </Button>
          <Button type="primary" onClick={showListVersion}>
            <b>{t('historyversion')}</b>
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
      {visibleSettingModal && (
        <FlowSettingModal
          id={id || ''}
          visible={visibleSettingModal}
          hideModal={() => setVisibleSettingMModal(false)}
        ></FlowSettingModal>
      )}
      {visibleHistoryVersionModal && (
        <HistoryVersionModal
          id={id || ''}
          visible={visibleHistoryVersionModal}
          hideModal={() => setVisibleHistoryVersionModal(false)}
        ></HistoryVersionModal>
      )}
    </>
  );
};

export default FlowHeader;
