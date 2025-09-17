import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { LLMFactory } from '@/constants/llm';
import { useTranslate } from '@/hooks/common-hooks';
import { Divider, Form, Input, InputNumber, Modal, Switch } from 'antd';
import { useEffect } from 'react';
import { ApiKeyPostBody } from '../../interface';

interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  loading: boolean;
  initialValue: string;
  llmFactory: string;
  onOk: (postBody: ApiKeyPostBody) => void;
  showModal?(): void;
}

type FieldType = {
  api_key?: string;
  base_url?: string;
  group_id?: string;
  // WhisperX specific configuration
  enable_diarization?: boolean;
  min_speakers?: number;
  max_speakers?: number;
  initial_prompt?: string;
  condition_on_previous_text?: boolean;
  diarization_batch_size?: number;

  // WhisperX ASR Options
  beam_size?: number;
  best_of?: number;
  vad_onset?: number;
  vad_offset?: number;
};

const modelsWithBaseUrl = [LLMFactory.OpenAI, LLMFactory.AzureOpenAI];

const ApiKeyModal = ({
  visible,
  hideModal,
  llmFactory,
  loading,
  initialValue,
  onOk,
}: IProps) => {
  const [form] = Form.useForm();
  const { t } = useTranslate('setting');

  const handleOk = async () => {
    const ret = await form.validateFields();

    return onOk(ret);
  };

  const handleKeyDown = async (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      await handleOk();
    }
  };

  useEffect(() => {
    if (visible) {
      form.setFieldValue('api_key', initialValue);

      // Set default values for WhisperX configuration
      if (llmFactory === LLMFactory.WhisperX) {
        form.setFieldsValue({
          enable_diarization: true,
          min_speakers: 1,
          max_speakers: 5,
          condition_on_previous_text: true,
          diarization_batch_size: 16,
          initial_prompt: '',
          beam_size: 5,
          best_of: 5,
          vad_onset: 0.5,
          vad_offset: 0.363,
        });
      }
    }
  }, [initialValue, form, visible, llmFactory]);

  return (
    <Modal
      title={t('modify')}
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      okButtonProps={{ loading }}
      confirmLoading={loading}
    >
      <Form
        name="basic"
        labelCol={{ span: 8 }}
        wrapperCol={{ span: 16 }}
        style={{ maxWidth: 600 }}
        autoComplete="off"
        form={form}
      >
        <Form.Item<FieldType>
          label={t('apiKey')}
          name="api_key"
          tooltip={
            llmFactory === LLMFactory.WhisperX
              ? 'API key not required for local WhisperX model'
              : t('apiKeyTip')
          }
          rules={[
            {
              required: llmFactory !== LLMFactory.WhisperX,
              message: t('apiKeyMessage'),
            },
          ]}
        >
          <Input
            onKeyDown={handleKeyDown}
            placeholder={
              llmFactory === LLMFactory.WhisperX
                ? 'Not required for local model'
                : undefined
            }
          />
        </Form.Item>
        {modelsWithBaseUrl.some((x) => x === llmFactory) && (
          <Form.Item<FieldType>
            label={t('baseUrl')}
            name="base_url"
            tooltip={t('baseUrlTip')}
          >
            <Input
              placeholder="https://api.openai.com/v1"
              onKeyDown={handleKeyDown}
            />
          </Form.Item>
        )}
        {llmFactory?.toLowerCase() === 'Minimax'.toLowerCase() && (
          <Form.Item<FieldType> label={'Group ID'} name="group_id">
            <Input />
          </Form.Item>
        )}
        {llmFactory === LLMFactory.WhisperX && (
          <>
            <Divider>WhisperX Configuration</Divider>
            <Form.Item<FieldType>
              label="Enable Diarization"
              name="enable_diarization"
              valuePropName="checked"
              tooltip="Enable speaker diarization to identify different speakers in the audio"
            >
              <Switch />
            </Form.Item>
            <Form.Item<FieldType>
              label="Previous Text Condition"
              name="condition_on_previous_text"
              valuePropName="checked"
              tooltip="Whether to condition the model on previous text for better continuity"
            >
              <Switch />
            </Form.Item>
            <Form.Item<FieldType>
              label="Initial Prompt"
              name="initial_prompt"
              tooltip="Optional initial prompt to guide the transcription"
            >
              <Input.TextArea
                rows={2}
                placeholder="Optional initial prompt for transcription context"
              />
            </Form.Item>

            <Divider>WhisperX ASR Options</Divider>
            <Form.Item<FieldType>
              label="Min Speakers"
              name="min_speakers"
              tooltip="Minimum number of speakers expected in the audio"
              rules={[{ required: true, message: 'Min speakers is required' }]}
            >
              <InputNumber min={1} max={10} />
            </Form.Item>
            <Form.Item<FieldType>
              label="Max Speakers"
              name="max_speakers"
              tooltip="Maximum number of speakers expected in the audio"
              rules={[{ required: true, message: 'Max speakers is required' }]}
            >
              <InputNumber min={1} max={20} />
            </Form.Item>
            <Form.Item<FieldType>
              label="Beam Size"
              name="beam_size"
              tooltip="Number of beams for beam search decoding (higher = more accurate but slower)"
              rules={[{ required: true, message: 'Beam size is required' }]}
            >
              <InputNumber min={1} max={20} />
            </Form.Item>
            <Form.Item<FieldType>
              label="Best Of"
              name="best_of"
              tooltip="Number of candidates to consider for best result selection"
              rules={[{ required: true, message: 'Best of is required' }]}
            >
              <InputNumber min={1} max={20} />
            </Form.Item>
            <Form.Item<FieldType>
              label="VAD Onset"
              name="vad_onset"
              tooltip="Voice Activity Detection onset threshold (lower = more sensitive to speech start)"
              rules={[{ required: true, message: 'VAD onset is required' }]}
            >
              <InputNumber min={0} max={1} step={0.001} />
            </Form.Item>
            <Form.Item<FieldType>
              label="VAD Offset"
              name="vad_offset"
              tooltip="Voice Activity Detection offset threshold (lower = more sensitive to speech end)"
              rules={[{ required: true, message: 'VAD offset is required' }]}
            >
              <InputNumber min={0} max={1} step={0.001} />
            </Form.Item>
            <Form.Item<FieldType>
              label="Diarization Batch Size"
              name="diarization_batch_size"
              tooltip="Batch size for diarization processing (affects memory usage and speed)"
              rules={[
                {
                  required: true,
                  message: 'Diarization batch size is required',
                },
              ]}
            >
              <InputNumber min={1} max={64} />
            </Form.Item>
          </>
        )}
      </Form>
    </Modal>
  );
};

export default ApiKeyModal;
