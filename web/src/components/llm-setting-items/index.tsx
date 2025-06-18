import {
  LlmModelType,
  ModelVariableType,
  settledModelVariableMap,
} from '@/constants/knowledge';
import { Flex, Form, InputNumber, Select, Slider, Switch, Tooltip } from 'antd';
import camelCase from 'lodash/camelCase';

import { useTranslate } from '@/hooks/common-hooks';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/llm-hooks';
import { setChatVariableEnabledFieldValuePage } from '@/utils/chat';
import { QuestionCircleOutlined } from '@ant-design/icons';
import { useCallback, useMemo } from 'react';
import styles from './index.less';

interface IProps {
  prefix?: string;
  formItemLayout?: any;
  handleParametersChange?(value: ModelVariableType): void;
  onChange?(value: string, option: any): void;
}

const LlmSettingItems = ({ prefix, formItemLayout = {}, onChange }: IProps) => {
  const form = Form.useFormInstance();
  const { t } = useTranslate('chat');
  const parameterOptions = Object.values(ModelVariableType).map((x) => ({
    label: t(camelCase(x)),
    value: x,
  }));

  const handleParametersChange = useCallback(
    (value: ModelVariableType) => {
      const variable = settledModelVariableMap[value];
      let nextVariable: Record<string, any> = variable;
      if (prefix) {
        nextVariable = { [prefix]: variable };
      }
      const variableCheckBoxFieldMap = setChatVariableEnabledFieldValuePage();
      form.setFieldsValue({ ...nextVariable, ...variableCheckBoxFieldMap });
    },
    [form, prefix],
  );

  const memorizedPrefix = useMemo(() => (prefix ? [prefix] : []), [prefix]);

  const modelOptions = useComposeLlmOptionsByModelTypes([
    LlmModelType.Chat,
    LlmModelType.Image2text,
  ]);

  return (
    <>
      <Form.Item
        label={t('model')}
        name="llm_id"
        tooltip={t('modelTip')}
        {...formItemLayout}
        rules={[{ required: true, message: t('modelMessage') }]}
      >
        <Select
          options={modelOptions}
          showSearch
          popupMatchSelectWidth={false}
          onChange={onChange}
        />
      </Form.Item>
      <div className="border rounded-md">
        <div className="flex justify-between bg-slate-100 p-2 mb-2">
          <div className="space-x-1 items-center">
            <span className="text-lg font-semibold">{t('freedom')}</span>
            <Tooltip title={t('freedomTip')}>
              <QuestionCircleOutlined></QuestionCircleOutlined>
            </Tooltip>
          </div>
          <div className="w-1/4 min-w-32">
            <Form.Item
              label={t('freedom')}
              name="parameter"
              tooltip={t('freedomTip')}
              initialValue={ModelVariableType.Precise}
              labelCol={{ span: 0 }}
              wrapperCol={{ span: 24 }}
              className="m-0"
            >
              <Select<ModelVariableType>
                options={parameterOptions}
                onChange={handleParametersChange}
              />
            </Form.Item>
          </div>
        </div>

        <div className="pr-2">
          <Form.Item
            label={t('temperature')}
            tooltip={t('temperatureTip')}
            {...formItemLayout}
          >
            <Flex gap={20} align="center">
              <Form.Item
                name={'temperatureEnabled'}
                valuePropName="checked"
                noStyle
              >
                <Switch size="small" />
              </Form.Item>
              <Form.Item
                noStyle
                dependencies={['temperatureEnabled']}
                shouldUpdate
              >
                {({ getFieldValue }) => {
                  const disabled = !getFieldValue('temperatureEnabled');
                  return (
                    <>
                      <Flex flex={1}>
                        <Form.Item
                          name={[...memorizedPrefix, 'temperature']}
                          noStyle
                        >
                          <Slider
                            className={styles.variableSlider}
                            max={1}
                            step={0.01}
                            disabled={disabled}
                          />
                        </Form.Item>
                      </Flex>
                      <Form.Item
                        name={[...memorizedPrefix, 'temperature']}
                        noStyle
                      >
                        <InputNumber
                          className={styles.sliderInputNumber}
                          max={1}
                          min={0}
                          step={0.01}
                          disabled={disabled}
                        />
                      </Form.Item>
                    </>
                  );
                }}
              </Form.Item>
            </Flex>
          </Form.Item>
          <Form.Item
            label={t('topP')}
            tooltip={t('topPTip')}
            {...formItemLayout}
          >
            <Flex gap={20} align="center">
              <Form.Item name={'topPEnabled'} valuePropName="checked" noStyle>
                <Switch size="small" />
              </Form.Item>
              <Form.Item noStyle dependencies={['topPEnabled']} shouldUpdate>
                {({ getFieldValue }) => {
                  const disabled = !getFieldValue('topPEnabled');
                  return (
                    <>
                      <Flex flex={1}>
                        <Form.Item name={[...memorizedPrefix, 'top_p']} noStyle>
                          <Slider
                            className={styles.variableSlider}
                            max={1}
                            step={0.01}
                            disabled={disabled}
                          />
                        </Form.Item>
                      </Flex>
                      <Form.Item name={[...memorizedPrefix, 'top_p']} noStyle>
                        <InputNumber
                          className={styles.sliderInputNumber}
                          max={1}
                          min={0}
                          step={0.01}
                          disabled={disabled}
                        />
                      </Form.Item>
                    </>
                  );
                }}
              </Form.Item>
            </Flex>
          </Form.Item>
          <Form.Item
            label={t('presencePenalty')}
            tooltip={t('presencePenaltyTip')}
            {...formItemLayout}
          >
            <Flex gap={20} align="center">
              <Form.Item
                name={'presencePenaltyEnabled'}
                valuePropName="checked"
                noStyle
              >
                <Switch size="small" />
              </Form.Item>
              <Form.Item
                noStyle
                dependencies={['presencePenaltyEnabled']}
                shouldUpdate
              >
                {({ getFieldValue }) => {
                  const disabled = !getFieldValue('presencePenaltyEnabled');
                  return (
                    <>
                      <Flex flex={1}>
                        <Form.Item
                          name={[...memorizedPrefix, 'presence_penalty']}
                          noStyle
                        >
                          <Slider
                            className={styles.variableSlider}
                            max={1}
                            step={0.01}
                            disabled={disabled}
                          />
                        </Form.Item>
                      </Flex>
                      <Form.Item
                        name={[...memorizedPrefix, 'presence_penalty']}
                        noStyle
                      >
                        <InputNumber
                          className={styles.sliderInputNumber}
                          max={1}
                          min={0}
                          step={0.01}
                          disabled={disabled}
                        />
                      </Form.Item>
                    </>
                  );
                }}
              </Form.Item>
            </Flex>
          </Form.Item>
          <Form.Item
            label={t('frequencyPenalty')}
            tooltip={t('frequencyPenaltyTip')}
            {...formItemLayout}
          >
            <Flex gap={20} align="center">
              <Form.Item
                name={'frequencyPenaltyEnabled'}
                valuePropName="checked"
                noStyle
              >
                <Switch size="small" />
              </Form.Item>
              <Form.Item
                noStyle
                dependencies={['frequencyPenaltyEnabled']}
                shouldUpdate
              >
                {({ getFieldValue }) => {
                  const disabled = !getFieldValue('frequencyPenaltyEnabled');
                  return (
                    <>
                      <Flex flex={1}>
                        <Form.Item
                          name={[...memorizedPrefix, 'frequency_penalty']}
                          noStyle
                        >
                          <Slider
                            className={styles.variableSlider}
                            max={1}
                            step={0.01}
                            disabled={disabled}
                          />
                        </Form.Item>
                      </Flex>
                      <Form.Item
                        name={[...memorizedPrefix, 'frequency_penalty']}
                        noStyle
                      >
                        <InputNumber
                          className={styles.sliderInputNumber}
                          max={1}
                          min={0}
                          step={0.01}
                          disabled={disabled}
                        />
                      </Form.Item>
                    </>
                  );
                }}
              </Form.Item>
            </Flex>
          </Form.Item>
          <Form.Item
            label={t('maxTokens')}
            tooltip={t('maxTokensTip')}
            {...formItemLayout}
          >
            <Flex gap={20} align="center">
              <Form.Item
                name={'maxTokensEnabled'}
                valuePropName="checked"
                noStyle
              >
                <Switch size="small" />
              </Form.Item>
              <Form.Item
                noStyle
                dependencies={['maxTokensEnabled']}
                shouldUpdate
              >
                {({ getFieldValue }) => {
                  const disabled = !getFieldValue('maxTokensEnabled');

                  return (
                    <>
                      <Flex flex={1}>
                        <Form.Item
                          name={[...memorizedPrefix, 'max_tokens']}
                          noStyle
                        >
                          <Slider
                            className={styles.variableSlider}
                            max={128000}
                            disabled={disabled}
                          />
                        </Form.Item>
                      </Flex>
                      <Form.Item
                        name={[...memorizedPrefix, 'max_tokens']}
                        noStyle
                      >
                        <InputNumber
                          disabled={disabled}
                          className={styles.sliderInputNumber}
                          max={128000}
                          min={0}
                        />
                      </Form.Item>
                    </>
                  );
                }}
              </Form.Item>
            </Flex>
          </Form.Item>
        </div>
      </div>
    </>
  );
};

export default LlmSettingItems;
