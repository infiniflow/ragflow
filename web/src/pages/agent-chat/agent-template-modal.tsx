/**
 * Agent模板选择对话框
 * 用于创建新Agent时，从预设模板中选择一个作为基础
 */
import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { useTranslate } from '@/hooks/common-hooks';
import { useFetchFlowTemplates } from '@/hooks/flow-hooks';
import { useSelectItem } from '@/hooks/logic-hooks';
import { Button, Card, Flex, List, Modal, Typography } from 'antd';
import DOMPurify from 'dompurify';
import { useCallback } from 'react';
import GraphAvatar from '../flow/list/graph-avatar';
import styles from './index.less';

const { Text, Title, Paragraph } = Typography;

/**
 * 模板选择对话框属性接口
 */
interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  hideModal: () => void;
  onTemplateSelect: (templateId: string) => void;
  visible: boolean;
}

const AgentTemplateModal = ({
  visible,
  hideModal,
  onTemplateSelect,
}: IProps) => {
  // 获取翻译函数
  const { t } = useTranslate('agent');
  const tFlow = useTranslate('flow').t;

  // 获取可用的工作流模板列表
  const { data: templateList } = useFetchFlowTemplates();

  // 模板选择状态
  const { selectedId, handleItemClick } = useSelectItem('');

  /**
   * 处理使用模板
   * 当用户点击"使用模板"按钮时触发
   */
  const handleUseTemplate = useCallback(
    (id: string) => () => {
      onTemplateSelect(id);
    },
    [onTemplateSelect],
  );

  return (
    <Modal
      title={t('createFromTemplate')}
      open={visible}
      width={'100vw'}
      onCancel={hideModal}
      className={styles.agentTemplateModal}
      wrapClassName={styles.agentTemplateModalWrapper}
      footer={null}
    >
      <section className={styles.createModalContent}>
        <Title level={5}>{t('templateSelection')}</Title>
        {/* 模板列表 */}
        <List
          grid={{ gutter: 16, column: 4 }}
          dataSource={templateList}
          renderItem={(template) => (
            <List.Item>
              {/* 模板卡片 */}
              <Card
                key={template.id}
                onMouseEnter={handleItemClick(template.id)}
                onMouseLeave={handleItemClick('')}
                className={styles.flowTemplateCard}
              >
                {/* 模板标题和图标 */}
                <Flex gap={'middle'} align="center">
                  <div className={styles.templateAvatar}>
                    <GraphAvatar avatar={template.avatar} />
                  </div>
                  <b className={styles.agentTitleWrapper}>
                    <Text
                      style={{ width: '96%' }}
                      ellipsis={{ tooltip: template.title }}
                    >
                      {template.title}
                    </Text>
                  </b>
                </Flex>
                {/* 模板描述 */}
                <div className={styles.agentDescription}>
                  <Paragraph
                    ellipsis={{ rows: 5, tooltip: template.description }}
                  >
                    <div
                      dangerouslySetInnerHTML={{
                        __html: DOMPurify.sanitize(template.description),
                      }}
                    ></div>
                  </Paragraph>
                </div>
                {/* 使用模板按钮，仅在鼠标悬停时显示 */}
                {selectedId === template.id && (
                  <Button
                    type={'primary'}
                    block
                    onClick={handleUseTemplate(template.id)}
                    className={styles.useButton}
                  >
                    {tFlow('useTemplate')}
                  </Button>
                )}
              </Card>
            </List.Item>
          )}
        />
      </section>
    </Modal>
  );
};

export default AgentTemplateModal;
