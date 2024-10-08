import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { IAddLlmRequestBody } from '@/interfaces/request/llm';
import { Flex, Form, Input, Modal, Select, Space, Switch } from 'antd';
import omit from 'lodash/omit';

type FieldType = IAddLlmRequestBody & { vision: boolean };

const { Option } = Select;

const llmFactoryToUrlMap = {
  Ollama:
    'https://github.com/infiniflow/ragflow/blob/main/docs/guides/deploy_local_llm.mdx',
  Xinference: 'https://inference.readthedocs.io/en/latest/user_guide',
  LocalAI: 'https://localai.io/docs/getting-started/models/',
  'LM-Studio': 'https://lmstudio.ai/docs/basics',
  'OpenAI-API-Compatible': 'https://platform.openai.com/docs/models/gpt-4',
  TogetherAI: 'https://docs.together.ai/docs/deployment-options',
  Replicate: 'https://replicate.com/docs/topics/deployments',
  OpenRouter: 'https://openrouter.ai/docs',
  HuggingFace:
    'https://huggingface.co/docs/text-embeddings-inference/quick_tour',
};
type LlmFactory = keyof typeof llmFactoryToUrlMap;

const OllamaModal = ({
  visible,
  hideModal,
  onOk,
  loading,
  llmFactory,
}: IModalProps<IAddLlmRequestBody> & { llmFactory: string }) => {
  const [form] = Form.useForm<FieldType>();

  const { t } = useTranslate('setting');

  const handleOk = async () => {
    const values = await form.validateFields();
    const modelType =
      values.model_type === 'chat' && values.vision
        ? 'image2text'
        : values.model_type;

    const data = {
      ...omit(values, ['vision']),
      model_type: modelType,
      llm_factory: llmFactory,
    };
    console.info(data);

    onOk?.(data);
  };
  const url =
    llmFactoryToUrlMap[llmFactory as LlmFactory] ||
    'https://github.com/infiniflow/ragflow/blob/main/docs/guides/deploy_local_llm.mdx';
  const optionsMap = {
    HuggingFace: [{ value: 'embedding', label: 'embedding' }],
    Xinference: [
      { value: 'chat', label: 'chat' },
      { value: 'embedding', label: 'embedding' },
      { value: 'rerank', label: 'rerank' },
      { value: 'image2text', label: 'image2text' },
      { value: 'speech2text', label: 'sequence2text' },
      { value: 'tts', label: 'tts' },
    ],
    Default: [
      { value: 'chat', label: 'chat' },
      { value: 'embedding', label: 'embedding' },
      { value: 'rerank', label: 'rerank' },
      { value: 'image2text', label: 'image2text' },
    ],
  };
  const getOptions = (factory: string) => {
    return optionsMap[factory as keyof typeof optionsMap] || optionsMap.Default;
  };
  return (
    <Modal
      title={t('addLlmTitle', { name: llmFactory })}
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      okButtonProps={{ loading }}
      footer={(originNode: React.ReactNode) => {
        return (
          <Flex justify={'space-between'}>
            <a href={url} target="_blank" rel="noreferrer">
              {t('ollamaLink', { name: llmFactory })}
            </a>
            <Space>{originNode}</Space>
          </Flex>
        );
      }}
    >
      <Form
        name="basic"
        style={{ maxWidth: 600 }}
        autoComplete="off"
        layout={'vertical'}
        form={form}
      >
        <Form.Item<FieldType>
          label={t('modelType')}
          name="model_type"
          initialValue={'chat'}
          rules={[{ required: true, message: t('modelTypeMessage') }]}
        >
          <Select placeholder={t('modelTypeMessage')}>
            {getOptions(llmFactory).map((option) => (
              <Option key={option.value} value={option.value}>
                {option.label}
              </Option>
            ))}
          </Select>
        </Form.Item>
        <Form.Item<FieldType>
          label={t(llmFactory === 'Xinference' ? 'modelUid' : 'modelName')}
          name="llm_name"
          rules={[{ required: true, message: t('modelNameMessage') }]}
        >
          <Input placeholder={t('modelNameMessage')} />
        </Form.Item>
        <Form.Item<FieldType>
          label={t('addLlmBaseUrl')}
          name="api_base"
          rules={[{ required: true, message: t('baseUrlNameMessage') }]}
        >
          <Input placeholder={t('baseUrlNameMessage')} />
        </Form.Item>
        <Form.Item<FieldType>
          label={t('apiKey')}
          name="api_key"
          rules={[{ required: false, message: t('apiKeyMessage') }]}
        >
          <Input placeholder={t('apiKeyMessage')} />
        </Form.Item>
        <Form.Item noStyle dependencies={['model_type']}>
          {({ getFieldValue }) =>
            getFieldValue('model_type') === 'chat' && (
              <Form.Item
                label={t('vision')}
                valuePropName="checked"
                name={'vision'}
              >
                <Switch />
              </Form.Item>
            )
          }
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default OllamaModal;
