import { useFetchKnowledgeGraph } from '@/hooks/chunk-hooks';
import { Modal } from 'antd';
import React, { useEffect, useState } from 'react';
import ForceGraph from './force-graph';

import styles from './index.less';

const KnowledgeGraphModal: React.FC = () => {
  const [isModalOpen, setIsModalOpen] = useState(false);
  const { data } = useFetchKnowledgeGraph();

  const handleOk = () => {
    setIsModalOpen(false);
  };

  const handleCancel = () => {
    setIsModalOpen(false);
  };

  useEffect(() => {
    if (data?.data && typeof data?.data !== 'boolean') {
      console.log('🚀 ~ useEffect ~ data:', data);
      setIsModalOpen(true);
    }
  }, [setIsModalOpen, data]);

  return (
    <Modal
      title="Basic Modal"
      open={isModalOpen}
      onOk={handleOk}
      onCancel={handleCancel}
      width={'90vw'}
      footer={null}
    >
      <section className={styles.modalContainer}>
        <ForceGraph></ForceGraph>
      </section>
    </Modal>
  );
};

export default KnowledgeGraphModal;
