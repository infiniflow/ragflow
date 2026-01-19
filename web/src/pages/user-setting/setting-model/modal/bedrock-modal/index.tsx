import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { IAddLlmRequestBody } from '@/interfaces/request/llm';
import {
  Form,
  Input,
  InputNumber,
  Modal,
  Segmented,
  Select,
  Typography,
} from 'antd';
import { useMemo, useState } from 'react';
import { LLMHeader } from '../../components/llm-header';
import { BedrockRegionList } from '../../constant';

type FieldType = IAddLlmRequestBody & {
  auth_mode?: 'access_key_secret' | 'iam_role' | 'assume_role';
  bedrock_ak: string;
  bedrock_sk: string;
  bedrock_region: string;
  aws_role_arn?: string;
};

const { Option } = Select;
const { Text } = Typography;

const BedrockModal = ({
  visible,
  hideModal,
  onOk,
  loading,
  llmFactory,
}: IModalProps<IAddLlmRequestBody> & { llmFactory: string }) => {
  const [form] = Form.useForm<FieldType>();
  const [authMode, setAuthMode] =
    useState<FieldType['auth_mode']>('access_key_secret');

  const { t } = useTranslate('setting');
  const options = useMemo(
    () => BedrockRegionList.map((x) => ({ value: x, label: t(x) })),
    [t],
  );

  const handleOk = async () => {
    const values = await form.validateFields();

    // Only submit fields related to the active auth mode.
    const cleanedValues: Record<string, any> = { ...values };

    const fieldsByMode: Record<string, string[]> = {
      access_key_secret: ['bedrock_ak', 'bedrock_sk'],
      iam_role: ['aws_role_arn'],
      assume_role: [],
    };

    cleanedValues.auth_mode = authMode;

    Object.keys(fieldsByMode).forEach((mode) => {
      if (mode !== authMode) {
        fieldsByMode[mode].forEach((field) => {
          delete cleanedValues[field];
        });
      }
    });

    const data = {
      ...cleanedValues,
      llm_factory: llmFactory,
      max_tokens: values.max_tokens,
    };

    onOk?.(data as unknown as IAddLlmRequestBody);
  };

  return (
    <Modal
      title={
        <div>
          <LLMHeader name={llmFactory} />
        </div>
      }
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      okButtonProps={{ loading }}
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
            <Option value="chat">chat</Option>
            <Option value="embedding">embedding</Option>
          </Select>
        </Form.Item>
        <Form.Item<FieldType>
          label={t('modelName')}
          name="llm_name"
          rules={[{ required: true, message: t('bedrockModelNameMessage') }]}
        >
          <Input placeholder={t('bedrockModelNameMessage')} />
        </Form.Item>

        {/* AWS Credential Mode Switch (AK/SK section only) */}
        <Form.Item>
          <Segmented
            block
            value={authMode}
            onChange={(v) => {
              const next = v as FieldType['auth_mode'];
              setAuthMode(next);
              // Clear non-active fields so they won't be validated/submitted by accident.
              if (next !== 'access_key_secret') {
                form.setFieldsValue({ bedrock_ak: '', bedrock_sk: '' } as any);
              }
              if (next !== 'iam_role') {
                form.setFieldsValue({ aws_role_arn: '' } as any);
              }
              if (next !== 'assume_role') {
                form.setFieldsValue({ role_arn: '' } as any);
              }
            }}
            options={[
              {
                label: t('awsAuthModeAccessKeySecret'),
                value: 'access_key_secret',
              },
              { label: t('awsAuthModeIamRole'), value: 'iam_role' },
              { label: t('awsAuthModeAssumeRole'), value: 'assume_role' },
            ]}
          />
        </Form.Item>

        {authMode === 'access_key_secret' && (
          <>
            <Form.Item<FieldType>
              label={t('awsAccessKeyId')}
              name="bedrock_ak"
              rules={[{ required: true, message: t('bedrockAKMessage') }]}
            >
              <Input placeholder={t('bedrockAKMessage')} />
            </Form.Item>
            <Form.Item<FieldType>
              label={t('awsSecretAccessKey')}
              name="bedrock_sk"
              rules={[{ required: true, message: t('bedrockSKMessage') }]}
            >
              <Input placeholder={t('bedrockSKMessage')} />
            </Form.Item>
          </>
        )}

        {authMode === 'iam_role' && (
          <Form.Item<FieldType>
            label={t('awsRoleArn')}
            name="aws_role_arn"
            rules={[{ required: true, message: t('awsRoleArnMessage') }]}
          >
            <Input placeholder={t('awsRoleArnMessage')} />
          </Form.Item>
        )}

        {authMode === 'assume_role' && (
          <Form.Item
            style={{ marginTop: -8, marginBottom: 16 }}
            // keep layout consistent with other modes
          >
            <Text type="secondary">{t('awsAssumeRoleTip')}</Text>
          </Form.Item>
        )}

        <Form.Item<FieldType>
          label={t('bedrockRegion')}
          name="bedrock_region"
          rules={[{ required: true, message: t('bedrockRegionMessage') }]}
        >
          <Select
            placeholder={t('bedrockRegionMessage')}
            options={options}
            allowClear
          ></Select>
        </Form.Item>
        <Form.Item<FieldType>
          label={t('maxTokens')}
          name="max_tokens"
          rules={[
            { required: true, message: t('maxTokensMessage') },
            {
              type: 'number',
              message: t('maxTokensInvalidMessage'),
            },
            ({}) => ({
              validator(_, value) {
                if (value < 0) {
                  return Promise.reject(new Error(t('maxTokensMinMessage')));
                }
                return Promise.resolve();
              },
            }),
          ]}
        >
          <InputNumber
            placeholder={t('maxTokensTip')}
            style={{ width: '100%' }}
          />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default BedrockModal;
