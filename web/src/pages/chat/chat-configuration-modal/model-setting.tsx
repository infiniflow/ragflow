import {
  LlmModelType,
  ModelVariableType,
  settledModelVariableMap,
} from '@/constants/knowledge';
import { Divider, Flex, Form, InputNumber, Select, Slider, Switch } from 'antd';
import classNames from 'classnames';
import camelCase from 'lodash/camelCase';
import { useEffect } from 'react';
import { ISegmentedContentProps } from '../interface';

import { useTranslate } from '@/hooks/commonHooks';
import { useFetchLlmList, useSelectLlmOptions } from '@/hooks/llmHooks';
import { variableEnabledFieldMap } from '../constants';
import styles from './index.less';

const ModelSetting = ({ show, form }: ISegmentedContentProps) => {
  const { t } = useTranslate('chat');
  const parameterOptions = Object.values(ModelVariableType).map((x) => ({
    label: t(camelCase(x)),
    value: x,
  }));

  const modelOptions = useSelectLlmOptions();

  const handleParametersChange = (value: ModelVariableType) => {
    const variable = settledModelVariableMap[value];
    form.setFieldsValue({ llm_setting: variable });
  };

  useEffect(() => {
    const values = Object.keys(variableEnabledFieldMap).reduce<
      Record<string, boolean>
    >((pre, field) => {
      pre[field] = true;
      return pre;
    }, {});
    form.setFieldsValue(values);
  }, [form]);

  useFetchLlmList(LlmModelType.Chat);

  return (
    <section
      className={classNames({
        [styles.segmentedHidden]: !show,
      })}
    >
      <Form.Item
        label={t('model')}
        name="llm_id"
        tooltip={t('modelTip')}
        rules={[{ required: true, message: t('modelMessage') }]}
      >
        <Select options={modelOptions} showSearch />
      </Form.Item>
      <Divider></Divider>
      <Form.Item
        label={t('freedom')}
        name="parameters"
        tooltip={t('freedomTip')}
        initialValue={ModelVariableType.Precise}
        // rules={[{ required: true, message: 'Please input!' }]}
      >
        <Select<ModelVariableType>
          options={parameterOptions}
          onChange={handleParametersChange}
        />
      </Form.Item>
      <Form.Item label={t('temperature')} tooltip={t('temperatureTip')}>
        <Flex gap={20} align="center">
          <Form.Item
            name={'temperatureEnabled'}
            valuePropName="checked"
            noStyle
          >
            <Switch size="small" />
          </Form.Item>
          <Flex flex={1}>
            <Form.Item
              name={['llm_setting', 'temperature']}
              noStyle
              rules={[{ required: true, message: t('temperatureMessage') }]}
            >
              <Slider className={styles.variableSlider} max={1} step={0.01} />
            </Form.Item>
          </Flex>
          <Form.Item
            name={['llm_setting', 'temperature']}
            noStyle
            rules={[{ required: true, message: t('temperatureMessage') }]}
          >
            <InputNumber
              className={styles.sliderInputNumber}
              max={1}
              min={0}
              step={0.01}
            />
          </Form.Item>
        </Flex>
      </Form.Item>
      <Form.Item label={t('topP')} tooltip={t('topPTip')}>
        <Flex gap={20} align="center">
          <Form.Item name={'topPEnabled'} valuePropName="checked" noStyle>
            <Switch size="small" />
          </Form.Item>
          <Flex flex={1}>
            <Form.Item
              name={['llm_setting', 'top_p']}
              noStyle
              rules={[{ required: true, message: t('topPMessage') }]}
            >
              <Slider className={styles.variableSlider} max={1} step={0.01} />
            </Form.Item>
          </Flex>
          <Form.Item
            name={['llm_setting', 'top_p']}
            noStyle
            rules={[{ required: true, message: t('topPMessage') }]}
          >
            <InputNumber
              className={styles.sliderInputNumber}
              max={1}
              min={0}
              step={0.01}
            />
          </Form.Item>
        </Flex>
      </Form.Item>
      <Form.Item label={t('presencePenalty')} tooltip={t('presencePenaltyTip')}>
        <Flex gap={20} align="center">
          <Form.Item
            name={'presencePenaltyEnabled'}
            valuePropName="checked"
            noStyle
          >
            <Switch size="small" />
          </Form.Item>
          <Flex flex={1}>
            <Form.Item
              name={['llm_setting', 'presence_penalty']}
              noStyle
              rules={[{ required: true, message: t('presencePenaltyMessage') }]}
            >
              <Slider className={styles.variableSlider} max={1} step={0.01} />
            </Form.Item>
          </Flex>
          <Form.Item
            name={['llm_setting', 'presence_penalty']}
            noStyle
            rules={[{ required: true, message: t('presencePenaltyMessage') }]}
          >
            <InputNumber
              className={styles.sliderInputNumber}
              max={1}
              min={0}
              step={0.01}
            />
          </Form.Item>
        </Flex>
      </Form.Item>
      <Form.Item
        label={t('frequencyPenalty')}
        tooltip={t('frequencyPenaltyTip')}
      >
        <Flex gap={20} align="center">
          <Form.Item
            name={'frequencyPenaltyEnabled'}
            valuePropName="checked"
            noStyle
          >
            <Switch size="small" />
          </Form.Item>
          <Flex flex={1}>
            <Form.Item
              name={['llm_setting', 'frequency_penalty']}
              noStyle
              rules={[
                { required: true, message: t('frequencyPenaltyMessage') },
              ]}
            >
              <Slider className={styles.variableSlider} max={1} step={0.01} />
            </Form.Item>
          </Flex>
          <Form.Item
            name={['llm_setting', 'frequency_penalty']}
            noStyle
            rules={[{ required: true, message: t('frequencyPenaltyMessage') }]}
          >
            <InputNumber
              className={styles.sliderInputNumber}
              max={1}
              min={0}
              step={0.01}
            />
          </Form.Item>
        </Flex>
      </Form.Item>
      <Form.Item label={t('maxTokens')} tooltip={t('maxTokensTip')}>
        <Flex gap={20} align="center">
          <Form.Item name={'maxTokensEnabled'} valuePropName="checked" noStyle>
            <Switch size="small" />
          </Form.Item>
          <Flex flex={1}>
            <Form.Item
              name={['llm_setting', 'max_tokens']}
              noStyle
              rules={[{ required: true, message: t('maxTokensMessage') }]}
            >
              <Slider className={styles.variableSlider} max={2048} />
            </Form.Item>
          </Flex>
          <Form.Item
            name={['llm_setting', 'max_tokens']}
            noStyle
            rules={[{ required: true, message: t('maxTokensMessage') }]}
          >
            <InputNumber
              className={styles.sliderInputNumber}
              max={2048}
              min={0}
            />
          </Form.Item>
        </Flex>
      </Form.Item>
    </section>
  );
};

export default ModelSetting;
