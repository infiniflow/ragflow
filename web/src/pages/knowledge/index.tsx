import ModalManager from '@/components/modal-manager';
import { useFetchKnowledgeList } from '@/hooks/knowledgeHook';
import { useSelectUserInfo } from '@/hooks/userSettingHook';
import { PlusOutlined, SearchOutlined } from '@ant-design/icons';
import { Button, Empty, Flex, Input, Space, Spin } from 'antd';
import KnowledgeCard from './knowledge-card';
import KnowledgeCreatingModal from './knowledge-creating-modal';

import { useTranslation } from 'react-i18next';
import { useSearchKnowledge, useSelectKnowledgeListByKeywords } from './hooks';
import styles from './index.less';

const KnowledgeList = () => {
  const { searchString, handleInputChange } = useSearchKnowledge();
  const { loading } = useFetchKnowledgeList();
  const list = useSelectKnowledgeListByKeywords(searchString);
  const userInfo = useSelectUserInfo();
  const { t } = useTranslation('translation', { keyPrefix: 'knowledgeList' });

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

          <ModalManager>
            {({ visible, hideModal, showModal }) => (
              <>
                <Button
                  type="primary"
                  icon={<PlusOutlined />}
                  onClick={() => {
                    showModal();
                  }}
                  className={styles.topButton}
                >
                  {t('createKnowledgeBase')}
                </Button>
                <KnowledgeCreatingModal
                  visible={visible}
                  hideModal={hideModal}
                ></KnowledgeCreatingModal>
              </>
            )}
          </ModalManager>
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
    </Flex>
  );
};

export default KnowledgeList;
