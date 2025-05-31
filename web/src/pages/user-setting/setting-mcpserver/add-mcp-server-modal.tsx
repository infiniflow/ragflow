import { useFetchMcpServerInfo } from '@/hooks/mcp-server-setting-hooks';
import { IModalProps } from '@/interfaces/common';
import { IMcpServerInfo, IMcpServerVariable, McpServerType } from '@/interfaces/database/mcp-server';
import { Editor } from '@monaco-editor/react';
import { Form, Input, message, Modal, Select } from 'antd';
import { camelCase } from 'lodash';
import { useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import McpServerVariable from './mcp-server-variable';

interface IProps extends IModalProps<IMcpServerInfo> {
  currentMcpServerId?: string;
}

const AddingMcpServerModal = ({
  visible,
  hideModal,
  loading,
  currentMcpServerId,
  onOk,
}: IProps) => {
  const { TextArea } = Input;
  const [form] = Form.useForm();
  const { t } = useTranslation();
  const { data: currentMcpServerInfo } = useFetchMcpServerInfo(currentMcpServerId);

  const serverTypeOptions = useMemo(() => {
    return [McpServerType.Sse, McpServerType.StreamableHttp].map((x) => ({
      label: t(`setting.mcpServerTypes.${camelCase(x)}`),
      value: x,
    }));
  }, [t]);

  type FieldType = {
    name: string;
    description?: string;
    serverType: McpServerType;
    url: string;
    serverVariables?: IMcpServerVariable[];
    headers: string;
  };

  useEffect(() => {
    if (visible) {
      if (currentMcpServerInfo) {
        const data: FieldType = {
          name: currentMcpServerInfo.name,
          description: currentMcpServerInfo.description,
          serverType: currentMcpServerInfo.server_type,
          url: currentMcpServerInfo.url,
          serverVariables: currentMcpServerInfo.variables || [],
          headers: JSON.stringify(currentMcpServerInfo.headers, null, 4),
        };

        form.setFieldsValue(data);
      } else {
        form.setFieldsValue({})
      }
    }
  }, [form, currentMcpServerInfo, visible]);

  const handleOk = async () => {
    let ret;

    try {
      ret = await form.validateFields();
    } catch {
      return;
    }

    let headerData;

    try {
      headerData = !!ret.headers ? JSON.parse(ret.headers) : {};
    } catch (e: any) {
      message.error(`${t('setting.mcpServerHeaderParseFailed')}: ${e.message}`);
      return;
    }

    const mcpServerData: IMcpServerInfo = {
      id: currentMcpServerId || "",
      name: ret.name,
      description: ret.description,
      server_type: ret.serverType,
      url: ret.url,
      headers: headerData,
    };

    if (ret.serverVariables) {
      mcpServerData.variables = ret.serverVariables.map((v: any) => ({
        name: v.name,
        key: v.key,
      }));
    } else {
      mcpServerData.variables = [];
    }

    return onOk?.(mcpServerData);
  };

  return (
    <Modal
      title={t('setting.add')}
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      okButtonProps={{ loading }}
      confirmLoading={loading}
    >
      <Form
        name="basic"
        labelCol={{ span: 6 }}
        wrapperCol={{ span: 18 }}
        autoComplete="off"
        form={form}
      >
        <Form.Item<FieldType>
          label={t('common.name')}
          name="name"
          rules={[{ required: true }]}
        >
          <Input />
        </Form.Item>
        <Form.Item<FieldType>
          label={t('setting.mcpServerDescription')}
          name="description"
          rules={[{ required: false }]}
        >
          <TextArea rows={4} />
        </Form.Item>
        <Form.Item<FieldType>
          label={t('setting.mcpServerType')}
          name="serverType"
          rules={[{ required: true }]}
        >
          <Select options={serverTypeOptions} />
        </Form.Item>
        <Form.Item<FieldType>
          label={t('setting.mcpServerUrl')}
          name="url"
          rules={[{ required: true }]}
        >
          <Input />
        </Form.Item>
        <Form.Item
          label={t('setting.mcpServerVariables')}
          name="serverVariables"
          tooltip={t('setting.mcpServerVariablesTip')}
          rules={[{ required: false }]}
        >
          <McpServerVariable />
        </Form.Item>
        <Form.Item<FieldType>
          label={t('setting.mcpServerHeaders')}
          name="headers"
          tooltip={t('setting.mcpServerHeadersTip')}
          rules={[{ required: false }]}
        >
          <Editor
            height={200}
            defaultLanguage="json"
            theme="vs-dark"
            options={
              {
                minimap: {
                  enabled: false
                }
              }
            }
          />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default AddingMcpServerModal;
