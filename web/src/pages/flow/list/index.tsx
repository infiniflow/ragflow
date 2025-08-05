import { PlusOutlined, SearchOutlined } from '@ant-design/icons';
import {
  Button,
  Divider,
  Empty,
  Flex,
  Input,
  Skeleton,
  Space,
  Spin,
} from 'antd';
import AgentTemplateModal from './agent-template-modal';
import FlowCard from './flow-card';
import { useFetchDataOnMount, useSaveFlow } from './hooks';

import { useTranslate } from '@/hooks/common-hooks';
import { useMemo } from 'react';
import InfiniteScroll from 'react-infinite-scroll-component';
import styles from './index.less';

const FlowList = () => {
  const {
    showFlowSettingModal,
    hideFlowSettingModal,
    flowSettingVisible,
    flowSettingLoading,
    templateList,
    onFlowOk,
  } = useSaveFlow();
  const { t } = useTranslate('flow');

  const {
    data,
    loading,
    searchString,
    handleInputChange,
    fetchNextPage,
    hasNextPage,
  } = useFetchDataOnMount();

  const nextList = useMemo(() => {
    const list =
      data?.pages?.flatMap((x) => (Array.isArray(x.kbs) ? x.kbs : [])) ?? [];
    return list;
  }, [data?.pages]);

  const total = useMemo(() => {
    return data?.pages.at(-1).total ?? 0;
  }, [data?.pages]);

  return (
    <Flex className={styles.flowListWrapper} vertical flex={1} gap={'large'}>
      <Flex justify={'end'}>
        <Space size={'large'}>
          <Input
            placeholder={t('searchAgentPlaceholder')}
            value={searchString}
            style={{ width: 220 }}
            allowClear
            onChange={handleInputChange}
            prefix={<SearchOutlined />}
          />
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={showFlowSettingModal}
          >
            {t('createGraph')}
          </Button>
        </Space>
      </Flex>

      <Spin spinning={loading}>
        <div
          id="scrollableDiv"
          style={{
            height: 'calc(100vh - 200px)',
            overflow: 'auto',
          }}
        >
          <InfiniteScroll
            dataLength={nextList?.length ?? 0}
            next={fetchNextPage}
            hasMore={hasNextPage}
            loader={<Skeleton avatar paragraph={{ rows: 1 }} active />}
            endMessage={
              !!total && <Divider plain>{t('noMoreData')} 🤐</Divider>
            }
            scrollableTarget="scrollableDiv"
            scrollThreshold="200px"
          >
            <Flex
              gap={'large'}
              wrap="wrap"
              className={styles.flowCardContainer}
            >
              {nextList.length > 0 ? (
                nextList.map((item) => {
                  return <FlowCard item={item} key={item.id}></FlowCard>;
                })
              ) : (
                <Empty className={styles.knowledgeEmpty}></Empty>
              )}
            </Flex>
          </InfiniteScroll>
        </div>
      </Spin>
      {flowSettingVisible && (
        <AgentTemplateModal
          visible={flowSettingVisible}
          onOk={onFlowOk}
          loading={flowSettingLoading}
          hideModal={hideFlowSettingModal}
          templateList={templateList}
        ></AgentTemplateModal>
      )}
    </Flex>
  );
};

export default FlowList;
