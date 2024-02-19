import { Divider, Flex, Form, InputNumber, Select, Slider } from 'antd';
import classNames from 'classnames';
import { ISegmentedContentProps } from './interface';

import styles from './index.less';

const { Option } = Select;

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
              name={['address', 'province']}
              noStyle
              rules={[{ required: true, message: 'Province is required' }]}
            >
              <Slider style={{ display: 'inline-block', width: '100%' }} />
            </Form.Item>
          </Flex>
          <Form.Item
            name={['address', 'street']}
            noStyle
            rules={[{ required: true, message: 'Street is required' }]}
          >
            <InputNumber
              style={{
                width: 50,
              }}
            />
          </Form.Item>
        </Flex>
      </Form.Item>
      <Form.Item label="Top P">
        <Flex gap={20}>
          <Flex flex={1}>
            <Form.Item
              name={['address', 'province']}
              noStyle
              rules={[{ required: true, message: 'Province is required' }]}
            >
              <Slider style={{ display: 'inline-block', width: '100%' }} />
            </Form.Item>
          </Flex>
          <Form.Item
            name={['address', 'street']}
            noStyle
            rules={[{ required: true, message: 'Street is required' }]}
          >
            <InputNumber
              style={{
                width: 50,
              }}
            />
          </Form.Item>
        </Flex>
      </Form.Item>
      <Form.Item label="Presence Penalty">
        <Flex gap={20}>
          <Flex flex={1}>
            <Form.Item
              name={['address', 'province']}
              noStyle
              rules={[{ required: true, message: 'Province is required' }]}
            >
              <Slider style={{ display: 'inline-block', width: '100%' }} />
            </Form.Item>
          </Flex>
          <Form.Item
            name={['address', 'street']}
            noStyle
            rules={[{ required: true, message: 'Street is required' }]}
          >
            <InputNumber
              style={{
                width: 50,
              }}
            />
          </Form.Item>
        </Flex>
      </Form.Item>
      <Form.Item label="Frequency Penalty">
        <Flex gap={20}>
          <Flex flex={1}>
            <Form.Item
              name={['address', 'province']}
              noStyle
              rules={[{ required: true, message: 'Province is required' }]}
            >
              <Slider style={{ display: 'inline-block', width: '100%' }} />
            </Form.Item>
          </Flex>
          <Form.Item
            name={['address', 'street']}
            noStyle
            rules={[{ required: true, message: 'Street is required' }]}
          >
            <InputNumber
              style={{
                width: 50,
              }}
            />
          </Form.Item>
        </Flex>
      </Form.Item>
      <Form.Item label="Max Tokens">
        <Flex gap={20}>
          <Flex flex={1}>
            <Form.Item
              name={['address', 'province']}
              noStyle
              rules={[{ required: true, message: 'Province is required' }]}
            >
              <Slider style={{ display: 'inline-block', width: '100%' }} />
            </Form.Item>
          </Flex>
          <Form.Item
            name={['address', 'street']}
            noStyle
            rules={[{ required: true, message: 'Street is required' }]}
          >
            <InputNumber
              style={{
                width: 50,
              }}
            />
          </Form.Item>
        </Flex>
      </Form.Item>
    </section>
  );
};

export default ModelSetting;
