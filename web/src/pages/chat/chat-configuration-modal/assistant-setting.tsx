import { Form, Input } from 'antd';

import classNames from 'classnames';
import { ISegmentedContentProps } from './interface';

import styles from './index.less';

const AssistantSetting = ({ show }: ISegmentedContentProps) => {
  return (
    <section
      className={classNames({
        [styles.segmentedHidden]: !show,
      })}
    >
      <Form.Item
        name={'name'}
        label="Assistant name"
        rules={[{ required: true }]}
      >
        <Input placeholder="e.g. Resume Jarvis" />
      </Form.Item>
      <Form.Item name={'avatar'} label="Assistant avatar">
        <Input />
      </Form.Item>
      <Form.Item name={'keywords'} label="Keywords">
        <Input.TextArea autoSize={{ minRows: 3 }} />
      </Form.Item>
      <Form.Item name={'opener'} label="Set an opener">
        <Input.TextArea autoSize={{ minRows: 5 }} />
      </Form.Item>
    </section>
  );
};

export default AssistantSetting;
