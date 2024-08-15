import { useFetchKnowledgeGraph } from '@/hooks/chunk-hooks';
import { Modal } from 'antd';
import { useTranslation } from 'react-i18next';
import IndentedTree from './indented-tree';

import { IModalProps } from '@/interfaces/common';

const IndentedTreeModal = ({
  documentId,
  visible,
  hideModal,
}: IModalProps<any> & { documentId: string }) => {
  const { data } = useFetchKnowledgeGraph(documentId);
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
        <IndentedTree data={data?.data?.mind_map} show></IndentedTree>
      </section>
    </Modal>
  );
};

export default IndentedTreeModal;
