import SimilaritySlider from '@/components/similarity-slider';
import { Button, Card, Divider, Flex, Form, Input } from 'antd';
import { FormInstance } from 'antd/lib';

import { useTranslate } from '@/hooks/commonHooks';
import { useOneNamespaceEffectsLoading } from '@/hooks/storeHooks';
import styles from './index.less';

type FieldType = {
  similarity_threshold?: number;
  vector_similarity_weight?: number;
  question: string;
};

interface IProps {
  form: FormInstance;
  handleTesting: () => Promise<any>;
}

const TestingControl = ({ form, handleTesting }: IProps) => {
  const question = Form.useWatch('question', { form, preserve: true });
  const loading = useOneNamespaceEffectsLoading('testingModel', [
    'testDocumentChunk',
  ]);
  const { t } = useTranslate('knowledgeDetails');

  const buttonDisabled =
    !question || (typeof question === 'string' && question.trim() === '');

  return (
    <section className={styles.testingControlWrapper}>
      <div>
        <b>{t('testing')}</b>
      </div>
      <p>{t('testingDescription')}</p>
      <Divider></Divider>
      <section>
        <Form name="testing" layout="vertical" form={form}>
          <SimilaritySlider isTooltipShown></SimilaritySlider>
          <Card size="small" title={t('testText')}>
            <Form.Item<FieldType>
              name={'question'}
              rules={[{ required: true, message: t('testTextPlaceholder') }]}
            >
              <Input.TextArea autoSize={{ minRows: 8 }}></Input.TextArea>
            </Form.Item>
            <Flex justify={'end'}>
              <Button
                type="primary"
                size="small"
                onClick={handleTesting}
                disabled={buttonDisabled}
                loading={loading}
              >
                {t('testingLabel')}
              </Button>
            </Flex>
          </Card>
        </Form>
      </section>
      {/* <section>
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
      </section> */}
    </section>
  );
};

export default TestingControl;
