import SimilaritySlider from '@/components/similarity-slider';
import { DeleteOutlined, HistoryOutlined } from '@ant-design/icons';
import {
  Button,
  Card,
  Divider,
  Flex,
  Form,
  Input,
  Slider,
  Space,
  Tag,
} from 'antd';
import { FormInstance } from 'antd/lib';

import styles from './index.less';

const list = [1, 2, 3];

type FieldType = {
  similarity_threshold?: number;
  vector_similarity_weight?: number;
  top_k?: number;
  question: string;
};

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
      <div>
        <b>Retrieval testing</b>
      </div>
      <p>Final step! After success, leave the rest to Infiniflow AI.</p>
      <Divider></Divider>
      <section>
        <Form
          name="testing"
          layout="vertical"
          form={form}
          initialValues={{
            top_k: 1024,
          }}
        >
          <SimilaritySlider></SimilaritySlider>
          <Form.Item<FieldType> label="Top k" name={'top_k'}>
            <Slider marks={{ 0: 0, 2048: 2048 }} max={2048} />
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
        <div className={styles.historyTitle}>
          <Space size={'middle'}>
            <HistoryOutlined className={styles.historyIcon} />
            <b>Test history</b>
          </Space>
        </div>
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
