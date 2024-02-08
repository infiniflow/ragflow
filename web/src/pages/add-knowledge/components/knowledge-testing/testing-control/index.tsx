import {
  Button,
  Card,
  Divider,
  Flex,
  Form,
  Input,
  Slider,
  SliderSingleProps,
  Space,
  Tag,
} from 'antd';

import { DeleteOutlined, HistoryOutlined } from '@ant-design/icons';
import { FormInstance } from 'antd/lib';
import styles from './index.less';

const list = [1, 2, 3];

const marks: SliderSingleProps['marks'] = {
  0: '0',
  100: '1',
};

type FieldType = {
  similarity_threshold?: number;
  vector_similarity_weight?: number;
  top_k?: number;
  question: string;
};

const formatter = (value: number | undefined) => {
  return typeof value === 'number' ? value / 100 : 0;
};

const tooltip = { formatter };

interface IProps {
  form: FormInstance;
  handleTesting: () => Promise<any>;
}

const TestingControl = ({ form, handleTesting }: IProps) => {
  const question = Form.useWatch('question', { form, preserve: true });

  const buttonDisabled =
    !question || (typeof question === 'string' && question.trim() === '');

  return (
    <section className={styles.testingControlWrapper}>
      <p>
        <b>Retrieval testing</b>
      </p>
      <p>Final step! After success, leave the rest to Infiniflow AI.</p>
      <Divider></Divider>
      <section>
        <Form
          name="testing"
          layout="vertical"
          form={form}
          initialValues={{
            similarity_threshold: 20,
            vector_similarity_weight: 30,
            top_k: 1024,
          }}
        >
          <Form.Item<FieldType>
            label="Similarity threshold"
            name={'similarity_threshold'}
          >
            <Slider marks={marks} defaultValue={0} tooltip={tooltip} />
          </Form.Item>
          <Form.Item<FieldType>
            label="Vector similarity weight"
            name={'vector_similarity_weight'}
          >
            <Slider marks={marks} defaultValue={0} tooltip={tooltip} />
          </Form.Item>
          <Form.Item<FieldType> label="Top k" name={'top_k'}>
            <Slider marks={{ 0: 0, 2048: 2048 }} defaultValue={0} max={2048} />
          </Form.Item>
          <Card size="small" title="Test text">
            <Form.Item<FieldType>
              name={'question'}
              rules={[
                { required: true, message: 'Please input your question!' },
              ]}
            >
              <Input.TextArea autoSize={{ minRows: 8 }}></Input.TextArea>
            </Form.Item>
            <Flex justify={'space-between'}>
              <Tag>10/200</Tag>
              <Button
                type="primary"
                size="small"
                onClick={handleTesting}
                disabled={buttonDisabled}
              >
                Testing
              </Button>
            </Flex>
          </Card>
        </Form>
      </section>
      <section>
        <p className={styles.historyTitle}>
          <Space size={'middle'}>
            <HistoryOutlined className={styles.historyIcon} />
            <b>Test history</b>
          </Space>
        </p>
        <Space
          direction="vertical"
          size={'middle'}
          className={styles.historyCardWrapper}
        >
          {list.map((x) => (
            <Card className={styles.historyCard} key={x}>
              <Flex justify={'space-between'} gap={'small'}>
                <span>{x}</span>
                <div className={styles.historyText}>
                  content dcjsjl snldsh svnodvn svnodrfn svjdoghdtbnhdo
                  sdvhodhbuid sldghdrlh
                </div>
                <Flex gap={'small'}>
                  <span>time</span>
                  <DeleteOutlined></DeleteOutlined>
                </Flex>
              </Flex>
            </Card>
          ))}
        </Space>
      </section>
    </section>
  );
};

export default TestingControl;
