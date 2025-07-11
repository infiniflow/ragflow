import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { useSetModalState, useTranslate } from '@/hooks/common-hooks';
import { IFlowTemplate } from '@/interfaces/database/flow';
// import { useFetchFlowTemplates } from '@/hooks/flow-hooks';
import { useSelectItem } from '@/hooks/logic-hooks';
import { Button, Card, Flex, List, Modal, Typography } from 'antd';
import { useCallback, useState } from 'react';
import CreateAgentModal from './create-agent-modal';
import GraphAvatar from './graph-avatar';

import DOMPurify from 'dompurify';
import styles from './index.less';

const { Title, Text, Paragraph } = Typography;
interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  loading: boolean;
  onOk: (name: string, templateId: string) => void;
  showModal?(): void;
  templateList: IFlowTemplate[];
}

const AgentTemplateModal = ({
  visible,
  hideModal,
  loading,
  onOk,
  templateList,
}: IProps) => {
  const { t } = useTranslate('common');
  // const { data: list } = useFetchFlowTemplates();
  const { selectedId, handleItemClick } = useSelectItem('');
  const [checkedId, setCheckedId] = useState<string>('');

  const {
    visible: creatingVisible,
    hideModal: hideCreatingModal,
    showModal: showCreatingModal,
  } = useSetModalState();

  const handleOk = useCallback(
    async (name: string) => {
      return onOk(name, checkedId);
    },
    [onOk, checkedId],
  );

  const onShowCreatingModal = useCallback(
    (id: string) => () => {
      showCreatingModal();
      setCheckedId(id);
    },
    [showCreatingModal],
  );

  return (
    <Modal
      title={t('createGraph', { keyPrefix: 'flow' })}
      open={visible}
      width={'100vw'}
      onCancel={hideModal}
      okButtonProps={{ loading }}
      confirmLoading={loading}
      className={styles.agentTemplateModal}
      wrapClassName={styles.agentTemplateModalWrapper}
      footer={null}
    >
      <section className={styles.createModalContent}>
        <Title level={5}>
          {t('createFromTemplates', { keyPrefix: 'flow' })}
        </Title>
        <List
          grid={{ gutter: 16, column: 4 }}
          dataSource={templateList}
          renderItem={(x) => (
            <List.Item>
              <Card
                key={x.id}
                onMouseEnter={handleItemClick(x.id)}
                onMouseLeave={handleItemClick('')}
                className={styles.flowTemplateCard}
              >
                <Flex gap={'middle'} align="center">
                  <GraphAvatar avatar={x.avatar}></GraphAvatar>
                  <b className={styles.agentTitleWrapper}>
                    <Text
                      style={{ width: '96%' }}
                      ellipsis={{ tooltip: x.title }}
                    >
                      {x.title}
                    </Text>
                  </b>
                </Flex>
                <div className={styles.agentDescription}>
                  <Paragraph ellipsis={{ tooltip: x.description, rows: 5 }}>
                    <div
                      dangerouslySetInnerHTML={{
                        __html: DOMPurify.sanitize(x.description),
                      }}
                    ></div>
                  </Paragraph>
                </div>
                {selectedId === x.id && (
                  <Button
                    type={'primary'}
                    block
                    onClick={onShowCreatingModal(x.id)}
                    className={styles.useButton}
                  >
                    {t('useTemplate', { keyPrefix: 'flow' })}
                  </Button>
                )}
              </Card>
            </List.Item>
          )}
        />
      </section>
      {creatingVisible && (
        <CreateAgentModal
          loading={loading}
          visible={creatingVisible}
          hideModal={hideCreatingModal}
          onOk={handleOk}
        ></CreateAgentModal>
      )}
    </Modal>
  );
};

export default AgentTemplateModal;
