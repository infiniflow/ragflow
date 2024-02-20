import { ReactComponent as FilterIcon } from '@/assets/filter.svg';
import ModalManager from '@/components/modal-manager';
import { PlusOutlined } from '@ant-design/icons';
import { Button, Flex, Space } from 'antd';
import KnowledgeCard from './knowledge-card';
import KnowledgeCreatingModal from './knowledge-creating-modal';

import { useFetchKnowledgeList } from '@/hooks/knowledgeHook';
import styles from './index.less';

const Knowledge = () => {
  const data = useFetchKnowledgeList();

  return (
    <div className={styles.knowledge}>
      <div className={styles.topWrapper}>
        <div>
          <span className={styles.title}>Welcome back, Zing</span>
          <p className={styles.description}>
            Which database are we going to use today?
          </p>
        </div>
        <Space size={'large'}>
          <Button icon={<FilterIcon />} className={styles.filterButton}>
            Filters
          </Button>
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
      <Flex gap="large" wrap="wrap">
        {data.map((item: any) => {
          return <KnowledgeCard item={item} key={item.name}></KnowledgeCard>;
        })}
      </Flex>
    </div>
  );
};

export default Knowledge;
