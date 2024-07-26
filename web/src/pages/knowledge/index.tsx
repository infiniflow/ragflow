import { useNextFetchKnowledgeList } from '@/hooks/knowledge-hooks';
import { useFetchUserInfo } from '@/hooks/user-setting-hooks';
import { PlusOutlined, SearchOutlined } from '@ant-design/icons';
import { Button, Empty, Flex, Input, Space, Spin } from 'antd';
import KnowledgeCard from './knowledge-card';
import KnowledgeCreatingModal from './knowledge-creating-modal';

import { useTranslation } from 'react-i18next';
import { useSaveKnowledge, useSearchKnowledge } from './hooks';
import styles from './index.less';

const KnowledgeList = () => {
  const { searchString, handleInputChange } = useSearchKnowledge();
  const { loading, list: data } = useNextFetchKnowledgeList();
  const list = data.filter((x) => x.name.includes(searchString));
  const { data: userInfo } = useFetchUserInfo();
  const { t } = useTranslation('translation', { keyPrefix: 'knowledgeList' });
  const {
    visible,
    hideModal,
    showModal,
    onCreateOk,
    loading: creatingLoading,
  } = useSaveKnowledge();

  return (
    <Flex className={styles.knowledge} vertical flex={1}>
      <div className={styles.topWrapper}>
        <div>
          <span className={styles.title}>
            {t('welcome')}, {userInfo.nickname}
          </span>
          <p className={styles.description}>{t('description')}</p>
        </div>
        <Space size={'large'}>
          <Input
            placeholder={t('searchKnowledgePlaceholder')}
            value={searchString}
            style={{ width: 220 }}
            allowClear
            onChange={handleInputChange}
            prefix={<SearchOutlined />}
          />

          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={showModal}
            className={styles.topButton}
          >
            {t('createKnowledgeBase')}
          </Button>
        </Space>
      </div>
      <Spin spinning={loading}>
        <Flex
          gap={'large'}
          wrap="wrap"
          className={styles.knowledgeCardContainer}
        >
          {list.length > 0 ? (
            list.map((item: any) => {
              return (
                <KnowledgeCard item={item} key={item.name}></KnowledgeCard>
              );
            })
          ) : (
            <Empty className={styles.knowledgeEmpty}></Empty>
          )}
        </Flex>
      </Spin>
      <KnowledgeCreatingModal
        loading={creatingLoading}
        visible={visible}
        hideModal={hideModal}
        onOk={onCreateOk}
      ></KnowledgeCreatingModal>
    </Flex>
  );
};

export default KnowledgeList;
