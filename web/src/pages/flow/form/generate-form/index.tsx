import LLMSelect from '@/components/llm-select';
import MessageHistoryWindowSizeItem from '@/components/message-history-window-size-item';
import { PromptEditor } from '@/components/prompt-editor';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Switch } from 'antd';
import { IOperatorForm } from '../../interface';
import LLMToolsSelect from '@/components/llm-tools-select';
import { useState } from 'react';
import LLMMcpServerSelect from '@/components/llm-mcp-server-select';
import McpInputVariable from './mcp-input-variable';

const GenerateForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslate('flow');

  const [isCurrentLlmSupportTools, setCurrentLlmSupportTools] = useState(false);

  const onLlmSelectChanged = (_: string, option: any) => {
    setCurrentLlmSupportTools(option.is_tools);

    if (!option.is_tools) {
      node!!.data.form.llm_enabled_tools = [];
      node!!.data.form.llm_enabled_mcp_servers = [];
    }
  };

  return (
    <Form
      name="basic"
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
      layout={'vertical'}
    >
      <Form.Item
        name={'llm_id'}
        label={t('model', { keyPrefix: 'chat' })}
        tooltip={t('modelTip', { keyPrefix: 'chat' })}
      >
        <LLMSelect onInitialValue={onLlmSelectChanged} onChange={onLlmSelectChanged}></LLMSelect>
      </Form.Item>
      <Form.Item
        name={['prompt']}
        label={t('systemPrompt')}
        initialValue={t('promptText')}
        tooltip={t('promptTip')}
        rules={[
          {
            required: true,
            message: t('promptMessage'),
          },
        ]}
      >
        {/* <Input.TextArea rows={8}></Input.TextArea> */}
        <PromptEditor></PromptEditor>
      </Form.Item>
      <Form.Item
        name={'llm_enabled_tools'}
        label={t('modelEnabledTools', { keyPrefix: 'chat' })}
        tooltip={t('modelEnabledToolsTip', { keyPrefix: 'chat' })}
      >
        <LLMToolsSelect disabled={!isCurrentLlmSupportTools}></LLMToolsSelect>
      </Form.Item>
      <Form.Item
        name={'llm_enabled_mcp_servers'}
        label={t('modelEnabledMcpServers', { keyPrefix: 'chat' })}
        tooltip={t('modelEnabledMcpServersTip', { keyPrefix: 'chat' })}
      >
        <LLMMcpServerSelect disabled={!isCurrentLlmSupportTools}></LLMMcpServerSelect>
      </Form.Item>
      <McpInputVariable name="mcp_server_variable_map" node={node!!} />
      <Form.Item
        name={['cite']}
        label={t('cite')}
        initialValue={true}
        valuePropName="checked"
        tooltip={t('citeTip')}
      >
        <Switch />
      </Form.Item>
      <MessageHistoryWindowSizeItem
        initialValue={12}
      ></MessageHistoryWindowSizeItem>
    </Form>
  );
};

export default GenerateForm;
