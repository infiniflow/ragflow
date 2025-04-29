/**
 * 创建Agent设置对话框组件
 * 用于设置新Agent的名称、描述和关联的知识库
 * 权限选项包括：私有（仅自己可见）和团队（所有团队成员可见）
 * 选择团队权限时，会在创建后自动获取最新团队成员列表并设置权限
 */
import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { LlmModelType } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import { useSelectKnowledgeOptions } from '@/hooks/knowledge-hooks';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/llm-hooks';
import { Form, Input, Modal, Radio, Select, Space, Typography } from 'antd';
import { useState } from 'react';

const { Text } = Typography;

/**
 * Agent设置对话框属性接口
 */
interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  loading: boolean; // 加载状态
  onOk: (
    name: string,
    description?: string,
    knowledgeIds?: string[],
    isPrivate?: boolean,
    modelId?: string,
  ) => void; // 确认回调
  showModal?(): void; // 显示对话框函数
}

const AgentSettingModal = ({ visible, hideModal, loading, onOk }: IProps) => {
  // 创建Form实例
  const [form] = Form.useForm();
  // 获取翻译函数
  const { t } = useTranslate('agent');
  // 获取知识库选项列表
  const knowledgeOptions = useSelectKnowledgeOptions();
  // 跟踪选中的知识库ID数组
  const [selectedKnowledgeIds, setSelectedKnowledgeIds] = useState<string[]>(
    [],
  );
  // 获取模型选项
  const modelOptions = useComposeLlmOptionsByModelTypes([
    LlmModelType.Chat,
    LlmModelType.Image2text,
  ]);

  /**
   * 表单字段类型定义
   */
  type FieldType = {
    name?: string; // Agent名称
    description?: string; // Agent描述
    knowledgeIds?: string[]; // 关联知识库ID列表
    permission?: 'private' | 'team'; // 权限类型：private-只有我，team-团队
    modelId?: string; // 选择的模型ID
  };

  /**
   * 处理表单提交
   * 验证表单并调用onOk回调
   * 当选择team权限时，会在创建Agent后通过agent-hooks.ts中
   * 的setCanvasPermissions函数获取最新团队成员并设置权限
   */
  const handleOk = async () => {
    const ret = await form.validateFields();
    // 将permission值转换为布尔值传给回调函数
    const isPrivate = ret.permission === 'private';
    return onOk(
      ret.name,
      ret.description,
      ret.knowledgeIds,
      isPrivate,
      ret.modelId,
    );
  };

  /**
   * 处理知识库选择变化
   * 更新选中的知识库ID数组
   */
  const handleKnowledgeChange = (values: string[]) => {
    setSelectedKnowledgeIds(values);
  };

  return (
    <Modal
      title={t('createAgent')}
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      okButtonProps={{ loading }}
      confirmLoading={loading}
    >
      {/* Agent创建表单 */}
      <Form
        name="basic"
        labelCol={{ span: 4 }}
        wrapperCol={{ span: 20 }}
        style={{ maxWidth: 600 }}
        autoComplete="off"
        form={form}
        initialValues={{ permission: 'private' }} // 默认选择"只有我"
      >
        {/* Agent名称输入框 */}
        <Form.Item<FieldType>
          label={t('name', { keyPrefix: 'common' })}
          name="name"
          rules={[
            {
              required: true,
              message: t('namePlaceholder', { keyPrefix: 'common' }),
            },
          ]}
        >
          <Input placeholder={t('namePlaceholder', { keyPrefix: 'common' })} />
        </Form.Item>

        {/* Agent描述输入框 */}
        <Form.Item<FieldType>
          label={t('description', { keyPrefix: 'common' })}
          name="description"
        >
          <Input
            placeholder={t('descriptionPlaceholder', { keyPrefix: 'common' })}
          />
        </Form.Item>

        {/* 聊天模型选择下拉框 */}
        <Form.Item<FieldType>
          label={t('model', { keyPrefix: 'chat' })}
          name="modelId"
          tooltip={t('modelTip', { keyPrefix: 'chat' })}
        >
          <Select
            options={modelOptions}
            showSearch
            popupMatchSelectWidth={false}
            placeholder={t('selectModel', { keyPrefix: 'common' })}
            optionFilterProp="label"
          />
        </Form.Item>

        {/* 知识库多选下拉框 */}
        <Form.Item<FieldType>
          label={t('knowledgeBase', { keyPrefix: 'common' })}
          name="knowledgeIds"
        >
          <Select
            mode="multiple"
            onChange={handleKnowledgeChange}
            options={knowledgeOptions}
            allowClear
            placeholder={t('selectKnowledgeBasePlaceholder', {
              keyPrefix: 'common',
            })}
            optionFilterProp="label"
            maxTagCount={3}
            maxTagTextLength={10}
          />
        </Form.Item>

        {/* 权限选择 - 选择团队权限时会在创建后获取最新团队成员并设置权限 */}
        <Form.Item<FieldType>
          label={<Space> {t('permission', { keyPrefix: 'common' })}</Space>}
          name="permission"
          rules={[
            {
              required: true,
              message: t('permissionRequired', { keyPrefix: 'common' }),
            },
          ]}
        >
          <Radio.Group>
            <Radio value="private">
              {t('private', { keyPrefix: 'common' })}
            </Radio>
            <Radio value="team">{t('team', { keyPrefix: 'common' })}</Radio>
          </Radio.Group>
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default AgentSettingModal;
