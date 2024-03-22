import {
  LlmModelType,
  ModelVariableType,
  settledModelVariableMap,
} from '@/constants/knowledge';
import { Divider, Flex, Form, InputNumber, Select, Slider, Switch } from 'antd';
import classNames from 'classnames';
import { useEffect } from 'react';
import { ISegmentedContentProps } from '../interface';

import { useFetchLlmList, useSelectLlmOptions } from '@/hooks/llmHooks';
import { variableEnabledFieldMap } from '../constants';
import styles from './index.less';

const ModelSetting = ({ show, form }: ISegmentedContentProps) => {
  const parameterOptions = Object.values(ModelVariableType).map((x) => ({
    label: x,
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
        label="Model"
        name="llm_id"
        tooltip="Large language chat model"
        rules={[{ required: true, message: 'Please select!' }]}
      >
        <Select options={modelOptions} showSearch />
      </Form.Item>
      <Divider></Divider>
      <Form.Item
        label="Freedom"
        name="parameters"
        tooltip="'Precise' means the LLM will be conservative and answer your question cautiously. 'Improvise' means the you want LLM talk much and freely. 'Balance' is between cautiously and freely."
        initialValue={ModelVariableType.Precise}
        // rules={[{ required: true, message: 'Please input!' }]}
      >
        <Select<ModelVariableType>
          options={parameterOptions}
          onChange={handleParametersChange}
        />
      </Form.Item>
      <Form.Item label="Temperature" tooltip={'This parameter controls the randomness of predictions by the model. A lower temperature makes the model more confident in its responses, while a higher temperature makes it more creative and diverse.'}>
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
              rules={[{ required: true, message: 'Temperature is required' }]}
            >
              <Slider className={styles.variableSlider} max={1} step={0.01} />
            </Form.Item>
          </Flex>
          <Form.Item
            name={['llm_setting', 'temperature']}
            noStyle
            rules={[{ required: true, message: 'Temperature is required' }]}
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
      <Form.Item label="Top P" tooltip={'Also known as “nucleus sampling,” this parameter sets a threshold to select a smaller set of words to sample from. It focuses on the most likely words, cutting off the less probable ones.'}>
        <Flex gap={20} align="center">
          <Form.Item name={'topPEnabled'} valuePropName="checked" noStyle>
            <Switch size="small" />
          </Form.Item>
          <Flex flex={1}>
            <Form.Item
              name={['llm_setting', 'top_p']}
              noStyle
              rules={[{ required: true, message: 'Top_p is required' }]}
            >
              <Slider className={styles.variableSlider} max={1} step={0.01} />
            </Form.Item>
          </Flex>
          <Form.Item
            name={['llm_setting', 'top_p']}
            noStyle
            rules={[{ required: true, message: 'Top_p is required' }]}
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
      <Form.Item label="Presence Penalty" tooltip={'This discourages the model from repeating the same information by penalizing words that have already appeared in the conversation.'}>
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
              rules={[
                { required: true, message: 'Presence Penalty is required' },
              ]}
            >
              <Slider className={styles.variableSlider} max={1} step={0.01} />
            </Form.Item>
          </Flex>
          <Form.Item
            name={['llm_setting', 'presence_penalty']}
            noStyle
            rules={[
              { required: true, message: 'Presence Penalty is required' },
            ]}
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
      <Form.Item label="Frequency Penalty" tooltip={'Similar to the presence penalty, this reduces the model’s tendency to repeat the same words frequently.'}>
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
                { required: true, message: 'Frequency Penalty is required' },
              ]}
            >
              <Slider className={styles.variableSlider} max={1} step={0.01} />
            </Form.Item>
          </Flex>
          <Form.Item
            name={['llm_setting', 'frequency_penalty']}
            noStyle
            rules={[
              { required: true, message: 'Frequency Penalty is required' },
            ]}
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
      <Form.Item label="Max Tokens" tooltip={'This sets the maximum length of the model’s output, measured in the number of tokens (words or pieces of words).'}>
        <Flex gap={20} align="center">
          <Form.Item name={'maxTokensEnabled'} valuePropName="checked" noStyle>
            <Switch size="small" />
          </Form.Item>
          <Flex flex={1}>
            <Form.Item
              name={['llm_setting', 'max_tokens']}
              noStyle
              rules={[{ required: true, message: 'Max Tokens is required' }]}
            >
              <Slider className={styles.variableSlider} max={2048} />
            </Form.Item>
          </Flex>
          <Form.Item
            name={['llm_setting', 'max_tokens']}
            noStyle
            rules={[{ required: true, message: 'Max Tokens is required' }]}
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
