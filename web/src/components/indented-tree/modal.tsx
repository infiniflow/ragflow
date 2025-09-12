import { useTranslation } from 'react-i18next';
import IndentedTree from './indented-tree';

import { useFetchKnowledgeGraph } from '@/hooks/knowledge-hooks';
import { IModalProps } from '@/interfaces/common';
import { Modal } from 'antd';

const IndentedTreeModal = ({
  visible,
  hideModal,
}: IModalProps<any> & { documentId: string }) => {
  const { data } = useFetchKnowledgeGraph();
  const { t } = useTranslation();

  return (
    <Modal
      title={t('chunk.mind')}
      open={visible}
      onCancel={hideModal}
      width={'90vw'}
      footer={null}
    >
      <section>
        <IndentedTree data={data?.mind_map} show></IndentedTree>
      </section>
    </Modal>
  );
};

export default IndentedTreeModal;
