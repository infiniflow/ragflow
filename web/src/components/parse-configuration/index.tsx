import { DocumentParserType } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import { PlusOutlined } from '@ant-design/icons';
import { Button, Flex, Form, Input, InputNumber, Slider, Switch } from 'antd';
import random from 'lodash/random';

export const excludedParseMethods = [
  DocumentParserType.Table,
  DocumentParserType.Resume,
  DocumentParserType.One,
  DocumentParserType.Picture,
  DocumentParserType.KnowledgeGraph,
  DocumentParserType.Qa,
  DocumentParserType.Tag,
];

export const showRaptorParseConfiguration = (
  parserId: DocumentParserType | undefined,
) => {
  return !excludedParseMethods.some((x) => x === parserId);
};

export const excludedTagParseMethods = [
  DocumentParserType.Table,
  DocumentParserType.KnowledgeGraph,
  DocumentParserType.Tag,
];

export const showTagItems = (parserId: DocumentParserType) => {
  return !excludedTagParseMethods.includes(parserId);
};

// The three types "table", "resume" and "one" do not display this configuration.
const ParseConfiguration = () => {
  const form = Form.useFormInstance();
  const { t } = useTranslate('knowledgeConfiguration');

  const handleGenerate = () => {
    form.setFieldValue(
      ['parser_config', 'raptor', 'random_seed'],
      random(10000),
    );
  };

  return (
    <>
      <Form.Item
        name={['parser_config', 'raptor', 'use_raptor']}
        label={t('useRaptor')}
        initialValue={false}
        valuePropName="checked"
        tooltip={t('useRaptorTip')}
      >
        <Switch />
      </Form.Item>
      <Form.Item
        shouldUpdate={(prevValues, curValues) =>
          prevValues.parser_config.raptor.use_raptor !==
          curValues.parser_config.raptor.use_raptor
        }
      >
        {({ getFieldValue }) => {
          const useRaptor = getFieldValue([
            'parser_config',
            'raptor',
            'use_raptor',
          ]);

          return (
            useRaptor && (
              <>
                <Form.Item
                  name={['parser_config', 'raptor', 'prompt']}
                  label={t('prompt')}
                  initialValue={t('promptText')}
                  tooltip={t('promptTip')}
                  rules={[
                    {
                      required: true,
                      message: t('promptMessage'),
                    },
                  ]}
                >
                  <Input.TextArea rows={8} />
                </Form.Item>
                <Form.Item label={t('maxToken')} tooltip={t('maxTokenTip')}>
                  <Flex gap={20} align="center">
                    <Flex flex={1}>
                      <Form.Item
                        name={['parser_config', 'raptor', 'max_token']}
                        noStyle
                        initialValue={256}
                        rules={[
                          {
                            required: true,
                            message: t('maxTokenMessage'),
                          },
                        ]}
                      >
                        <Slider max={2048} style={{ width: '100%' }} />
                      </Form.Item>
                    </Flex>
                    <Form.Item
                      name={['parser_config', 'raptor', 'max_token']}
                      noStyle
                      rules={[
                        {
                          required: true,
                          message: t('maxTokenMessage'),
                        },
                      ]}
                    >
                      <InputNumber max={2048} min={0} />
                    </Form.Item>
                  </Flex>
                </Form.Item>
                <Form.Item label={t('threshold')} tooltip={t('thresholdTip')}>
                  <Flex gap={20} align="center">
                    <Flex flex={1}>
                      <Form.Item
                        name={['parser_config', 'raptor', 'threshold']}
                        noStyle
                        initialValue={0.1}
                        rules={[
                          {
                            required: true,
                            message: t('thresholdMessage'),
                          },
                        ]}
                      >
                        <Slider
                          min={0}
                          max={1}
                          style={{ width: '100%' }}
                          step={0.01}
                        />
                      </Form.Item>
                    </Flex>
                    <Form.Item
                      name={['parser_config', 'raptor', 'threshold']}
                      noStyle
                      rules={[
                        {
                          required: true,
                          message: t('thresholdMessage'),
                        },
                      ]}
                    >
                      <InputNumber max={1} min={0} step={0.01} />
                    </Form.Item>
                  </Flex>
                </Form.Item>
                <Form.Item label={t('maxCluster')} tooltip={t('maxClusterTip')}>
                  <Flex gap={20} align="center">
                    <Flex flex={1}>
                      <Form.Item
                        name={['parser_config', 'raptor', 'max_cluster']}
                        noStyle
                        initialValue={64}
                        rules={[
                          {
                            required: true,
                            message: t('maxClusterMessage'),
                          },
                        ]}
                      >
                        <Slider min={1} max={1024} style={{ width: '100%' }} />
                      </Form.Item>
                    </Flex>
                    <Form.Item
                      name={['parser_config', 'raptor', 'max_cluster']}
                      noStyle
                      rules={[
                        {
                          required: true,
                          message: t('maxClusterMessage'),
                        },
                      ]}
                    >
                      <InputNumber max={1024} min={1} />
                    </Form.Item>
                  </Flex>
                </Form.Item>
                <Form.Item label={t('randomSeed')}>
                  <Flex gap={20} align="center">
                    <Flex flex={1}>
                      <Form.Item
                        name={['parser_config', 'raptor', 'random_seed']}
                        noStyle
                        initialValue={0}
                        rules={[
                          {
                            required: true,
                            message: t('randomSeedMessage'),
                          },
                        ]}
                      >
                        <InputNumber style={{ width: '100%' }} />
                      </Form.Item>
                    </Flex>
                    <Form.Item noStyle>
                      <Button type="primary" onClick={handleGenerate}>
                        <PlusOutlined />
                      </Button>
                    </Form.Item>
                  </Flex>
                </Form.Item>
              </>
            )
          );
        }}
      </Form.Item>
    </>
  );
};

export default ParseConfiguration;
