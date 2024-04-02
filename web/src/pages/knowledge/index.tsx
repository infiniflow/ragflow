import ModalManager from '@/components/modal-manager';
import { useFetchKnowledgeList } from '@/hooks/knowledgeHook';
import { useSelectUserInfo } from '@/hooks/userSettingHook';
import { PlusOutlined } from '@ant-design/icons';
import { Button, Empty, Flex, Space, Spin } from 'antd';
import KnowledgeCard from './knowledge-card';
import KnowledgeCreatingModal from './knowledge-creating-modal';

import styles from './index.less';

const Knowledge = () => {
  const { list, loading } = useFetchKnowledgeList();
  const userInfo = useSelectUserInfo();

  return (
    <Flex className={styles.knowledge} vertical flex={1}>
      <div className={styles.topWrapper}>
        <div>
          <span className={styles.title}>
            Welcome back, {userInfo.nickname}
          </span>
          <p className={styles.description}>
            Which database are we going to use today?
          </p>
        </div>
        <Space size={'large'}>
          {/* <Button icon={<FilterIcon />} className={styles.filterButton}>
            Filters
          </Button> */}
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
                  Create knowledge base
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

export default Knowledge;
