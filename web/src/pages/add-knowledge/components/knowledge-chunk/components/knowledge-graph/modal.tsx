import { useFetchKnowledgeGraph } from '@/hooks/chunk-hooks';
import { Modal } from 'antd';
import React, { useEffect, useState } from 'react';
import ForceGraph from './force-graph';

import styles from './index.less';
import { isDataExist } from './util';

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
    if (isDataExist(data)) {
      setIsModalOpen(true);
    }
  }, [setIsModalOpen, data]);

  return (
    <Modal
      title="Knowledge Graph"
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
