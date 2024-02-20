import { Divider, Flex, Form, InputNumber, Select, Slider } from 'antd';
import classNames from 'classnames';
import { ISegmentedContentProps } from './interface';

import styles from './index.less';

const ModelSetting = ({ show }: ISegmentedContentProps) => {
  return (
    <section
      className={classNames({
        [styles.segmentedHidden]: !show,
      })}
    >
      <Form.Item
        label="Model"
        name="model"
        // rules={[{ required: true, message: 'Please input!' }]}
      >
        <Select />
      </Form.Item>
      <Divider></Divider>
      <Form.Item
        label="Parameters"
        name="parameters"
        // rules={[{ required: true, message: 'Please input!' }]}
      >
        <Select />
      </Form.Item>
      <Form.Item label="Temperature">
        <Flex gap={20}>
          <Flex flex={1}>
            <Form.Item
              name={'temperature'}
              noStyle
              rules={[{ required: true, message: 'Province is required' }]}
            >
              <Slider className={styles.variableSlider} max={1} step={0.01} />
            </Form.Item>
          </Flex>
          <Form.Item
            name={'temperature'}
            noStyle
            rules={[{ required: true, message: 'Street is required' }]}
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
      <Form.Item label="Top P">
        <Flex gap={20}>
          <Flex flex={1}>
            <Form.Item
              name={'top_p'}
              noStyle
              rules={[{ required: true, message: 'Province is required' }]}
            >
              <Slider className={styles.variableSlider} max={1} step={0.01} />
            </Form.Item>
          </Flex>
          <Form.Item
            name={'top_p'}
            noStyle
            rules={[{ required: true, message: 'Street is required' }]}
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
      <Form.Item label="Presence Penalty">
        <Flex gap={20}>
          <Flex flex={1}>
            <Form.Item
              name={'presence_penalty'}
              noStyle
              rules={[{ required: true, message: 'Province is required' }]}
            >
              <Slider className={styles.variableSlider} max={1} step={0.01} />
            </Form.Item>
          </Flex>
          <Form.Item
            name={'presence_penalty'}
            noStyle
            rules={[{ required: true, message: 'Street is required' }]}
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
      <Form.Item label="Frequency Penalty">
        <Flex gap={20}>
          <Flex flex={1}>
            <Form.Item
              name={'frequency_penalty'}
              noStyle
              rules={[{ required: true, message: 'Province is required' }]}
            >
              <Slider className={styles.variableSlider} max={1} step={0.01} />
            </Form.Item>
          </Flex>
          <Form.Item
            name={'frequency_penalty'}
            noStyle
            rules={[{ required: true, message: 'Street is required' }]}
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
      <Form.Item label="Max Tokens">
        <Flex gap={20}>
          <Flex flex={1}>
            <Form.Item
              name={'max_tokens'}
              noStyle
              rules={[{ required: true, message: 'Province is required' }]}
            >
              <Slider
                className={styles.variableSlider}
                defaultValue={0}
                max={2048}
              />
            </Form.Item>
          </Flex>
          <Form.Item
            name={'max_tokens'}
            noStyle
            rules={[{ required: true, message: 'Street is required' }]}
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
