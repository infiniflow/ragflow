import { useFetchKnowledgeGraph } from '@/hooks/chunk-hooks';
import { Flex, Modal, Segmented } from 'antd';
import React, { useEffect, useState } from 'react';
import ForceGraph from './force-graph';
import IndentedTree from './indented-tree';
import styles from './index.less';
import { isDataExist } from './util';

enum SegmentedValue {
  Graph = 'Graph',
  Mind = 'Mind',
}

const KnowledgeGraphModal: React.FC = () => {
  const [isModalOpen, setIsModalOpen] = useState(false);
  const { data } = useFetchKnowledgeGraph();
  const [value, setValue] = useState<SegmentedValue>(SegmentedValue.Graph);

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
        <Flex justify="end">
          <Segmented
            size="large"
            options={[SegmentedValue.Graph, SegmentedValue.Mind]}
            value={value}
            onChange={(v) => setValue(v as SegmentedValue)}
          />
        </Flex>
        <ForceGraph
          data={data?.data?.graph}
          show={value === SegmentedValue.Graph}
        ></ForceGraph>
        <IndentedTree
          data={data?.data?.mind_map}
          show={value === SegmentedValue.Mind}
        ></IndentedTree>
      </section>
    </Modal>
  );
};

export default KnowledgeGraphModal;
