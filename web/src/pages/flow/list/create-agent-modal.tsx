import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Input, Modal, Select, Space, Tooltip, Radio, Button } from 'antd';
import { QuestionCircleOutlined } from '@ant-design/icons';
// 导入 AgentTemplateModal 组件
import AgentTemplateModal from './agent-template-modal';
// 导入权限管理组件
import PermissionManagement from '@/pages/add-knowledge/components/knowledge-setting/configuration/permission-management';
// 导入相关钩子和方法
import { useState } from 'react';
import { useSaveFlow } from './hooks';
import { useSelectKnowledgeOptions } from '@/hooks/knowledge-hooks';

import styles from './styles.less';

interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  loading: boolean;
  onOk: (values: { name: string; kb_ids: string[]; collaborators?: any[] }) => void;
  showModal?(): void;
}

type FieldType = {
  name: string;
  kb_ids: string[];
  collaborators?: any[];  // 协作者字段，用于存储权限配置信息
};

const CreateAgentModal = ({ visible, hideModal, loading, onOk }: IProps) => {
  const [form] = Form.useForm<FieldType>();
  const { t } = useTranslate('common');
  const options = useSelectKnowledgeOptions();
  
  // 添加模板弹窗相关状态
  const [flowSettingVisible, setFlowSettingVisible] = useState(false);
  const [flowSettingLoading, setFlowSettingLoading] = useState(false);
  
  // 添加权限配置模态框状态
  const [permissionModalVisible, setPermissionModalVisible] = useState(false);
  
  // 创建显示和隐藏函数
  const showFlowSettingModal = () => setFlowSettingVisible(true);
  const hideFlowSettingModal = () => setFlowSettingVisible(false);
  
  // 显示和隐藏权限配置模态框
  const showPermissionModal = () => setPermissionModalVisible(true);
  const hidePermissionModal = () => setPermissionModalVisible(false);
  
  // 处理权限配置更新
  const handlePermissionChange = (collaborators: any[]) => {
    form.setFieldsValue({ collaborators });
    hidePermissionModal();
  };

  const handleOk = async () => {
    const values = await form.validateFields();
    return onOk(values);
  };

  // 处理流程设置表单提交
  const onFlowOk = (name: string, templateId: string) => {
    // 这里需要添加对应的逻辑，根据实际需求实现
    // 可能需要调用useSaveFlow的相关方法
    return Promise.resolve();
  };

  return (
    <Modal
      title={t('createGraph', { keyPrefix: 'flow' })}
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      okButtonProps={{ loading }}
      confirmLoading={loading}
      width={500}
      className={styles.createAgentModal}
    >
      <Form
        name="basic"
        layout="vertical"
        autoComplete="off"
        form={form}
        initialValues={{ permission: 'me' }}
      >
        <Form.Item<FieldType>
          label={<span className={styles.formLabel}>
            <span className={styles.required}>*</span> {t('name')}
          </span>}
          name="name"
          required={false} // 添加这一行，禁用自动星号  
          rules={[{ required: true, message: t('namePlaceholder') }]}
        >
          <Input placeholder={t('namePlaceholder')} />
        </Form.Item>

        <Form.Item<FieldType>
          label={
            <span className={styles.formLabel}>
              <span className={styles.required}>*</span> 权限
              <Tooltip title={t('permissionsTip', { keyPrefix: 'knowledgeConfiguration' })}>
                <QuestionCircleOutlined className={styles.tooltipIcon} />
              </Tooltip>
            </span>
          }
          name="collaborators"
          required={false}
        >
          <div className={styles.permissionButton}>
            <Button type="primary" onClick={showPermissionModal}>
              配置权限
            </Button>
            {form.getFieldValue('collaborators')?.length > 0 && (
              <span className={styles.permissionHint}>
                已配置 {form.getFieldValue('collaborators').length} 个协作者
              </span>
            )}
          </div>
        </Form.Item>

        <Form.Item<FieldType>
          label={t('knowledgeBases')}
          name="kb_ids"
          rules={[{ required: true, message: t('selectKnowledgeBase') }]}
        >
          <Select
            mode="multiple"
            options={options}
          />
        </Form.Item>
      </Form>
      
      {/* 只有当flowSettingVisible为true时才渲染AgentTemplateModal */}
      {flowSettingVisible && (
        <AgentTemplateModal
          visible={flowSettingVisible}
          onOk={onFlowOk}
          loading={flowSettingLoading}
          hideModal={hideFlowSettingModal}
        />
      )}
      
      {/* 权限配置模态框 */}
      <Modal
        title="权限配置"
        open={permissionModalVisible}
        onCancel={hidePermissionModal}
        width={700}
        footer={[
          <Button key="cancel" onClick={hidePermissionModal}>
            取消
          </Button>,
          <Button 
            key="submit" 
            type="primary" 
            onClick={() => {
              const currentCollaborators = form.getFieldValue('collaborators') || [];
              handlePermissionChange(currentCollaborators);
            }}
          >
            确定
          </Button>,
        ]}
      >
        <PermissionManagement
          value={form.getFieldValue('collaborators')}
          onChange={(collaborators) => form.setFieldsValue({ collaborators })}
        />
      </Modal>
    </Modal>
  );
};

export default CreateAgentModal;
