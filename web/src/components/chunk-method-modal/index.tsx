import MaxTokenNumber from '@/components/max-token-number';
import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { IKnowledgeFileParserConfig } from '@/interfaces/database/knowledge';
import { IChangeParserConfigRequestBody } from '@/interfaces/request/document';
import {
  MinusCircleOutlined,
  PlusOutlined,
  QuestionCircleOutlined,
} from '@ant-design/icons';
import {
  Button,
  Divider,
  Form,
  InputNumber,
  Modal,
  Space,
  Switch,
  Tag,
  Tooltip,
} from 'antd';
import omit from 'lodash/omit';
import React, { useEffect, useMemo } from 'react';
import { useFetchParserListOnMount } from './hooks';

import { useTranslate } from '@/hooks/commonHooks';
import styles from './index.less';

const { CheckableTag } = Tag;

interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  loading: boolean;
  onOk: (
    parserId: string,
    parserConfig: IChangeParserConfigRequestBody,
  ) => void;
  showModal?(): void;
  parserId: string;
  parserConfig: IKnowledgeFileParserConfig;
  documentExtension: string;
  documentId: string;
}

const hidePagesChunkMethods = ['qa', 'table', 'picture', 'resume', 'one'];

const ChunkMethodModal: React.FC<IProps> = ({
  documentId,
  parserId,
  onOk,
  hideModal,
  visible,
  documentExtension,
  parserConfig,
}) => {
  const { parserList, handleChange, selectedTag } = useFetchParserListOnMount(
    documentId,
    parserId,
    documentExtension,
  );
  const [form] = Form.useForm();
  const { t } = useTranslate('knowledgeDetails');

  const handleOk = async () => {
    const values = await form.validateFields();
    const parser_config = {
      ...values.parser_config,
      pages: values.pages?.map((x: any) => [x.from, x.to]) ?? [],
    };
    onOk(selectedTag, parser_config);
  };

  const isPdf = documentExtension === 'pdf';

  const showPages = useMemo(() => {
    return isPdf && hidePagesChunkMethods.every((x) => x !== selectedTag);
  }, [selectedTag, isPdf]);

  const showOne = useMemo(() => {
    return (
      isPdf &&
      hidePagesChunkMethods
        .filter((x) => x !== 'one')
        .every((x) => x !== selectedTag)
    );
  }, [selectedTag, isPdf]);

  const showMaxTokenNumber = selectedTag === 'naive';

  const hideDivider = [showPages, showOne, showMaxTokenNumber].every(
    (x) => x === false,
  );

  const afterClose = () => {
    form.resetFields();
  };

  useEffect(() => {
    if (visible) {
      const pages =
        parserConfig.pages?.map((x) => ({ from: x[0], to: x[1] })) ?? [];
      form.setFieldsValue({
        pages: pages.length > 0 ? pages : [{ from: 1, to: 1024 }],
        parser_config: omit(parserConfig, 'pages'),
      });
    }
  }, [form, parserConfig, visible]);

  return (
    <Modal
      title={t('chunkMethod')}
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      afterClose={afterClose}
    >
      <Space size={[0, 8]} wrap>
        <div className={styles.tags}>
          {parserList.map((x) => {
            return (
              <CheckableTag
                key={x.value}
                checked={selectedTag === x.value}
                onChange={(checked) => {
                  handleChange(x.value, checked);
                }}
              >
                {x.label}
              </CheckableTag>
            );
          })}
        </div>
      </Space>
      {hideDivider || <Divider></Divider>}
      <Form name="dynamic_form_nest_item" autoComplete="off" form={form}>
        {showPages && (
          <>
            <Space>
              <p>Page Ranges:</p>
              <Tooltip
                title={
                  'page ranges: Define the page ranges that need to be parsed. The pages that not included in these ranges will be ignored.'
                }
              >
                <QuestionCircleOutlined
                  className={styles.questionIcon}
                ></QuestionCircleOutlined>
              </Tooltip>
            </Space>
            <Form.List name="pages">
              {(fields, { add, remove }) => (
                <>
                  {fields.map(({ key, name, ...restField }) => (
                    <Space
                      key={key}
                      style={{
                        display: 'flex',
                      }}
                      align="baseline"
                    >
                      <Form.Item
                        {...restField}
                        name={[name, 'from']}
                        dependencies={name > 0 ? [name - 1, 'to'] : []}
                        rules={[
                          {
                            required: true,
                            message: 'Missing start page number',
                          },
                          ({ getFieldValue }) => ({
                            validator(_, value) {
                              if (
                                name === 0 ||
                                !value ||
                                getFieldValue(['pages', name - 1, 'to']) < value
                              ) {
                                return Promise.resolve();
                              }
                              return Promise.reject(
                                new Error(
                                  'The current value must be greater than the previous to!',
                                ),
                              );
                            },
                          }),
                        ]}
                      >
                        <InputNumber
                          placeholder="from"
                          min={0}
                          precision={0}
                          className={styles.pageInputNumber}
                        />
                      </Form.Item>
                      <Form.Item
                        {...restField}
                        name={[name, 'to']}
                        dependencies={[name, 'from']}
                        rules={[
                          {
                            required: true,
                            message: 'Missing end page number(excluded)',
                          },
                          ({ getFieldValue }) => ({
                            validator(_, value) {
                              if (
                                !value ||
                                getFieldValue(['pages', name, 'from']) < value
                              ) {
                                return Promise.resolve();
                              }
                              return Promise.reject(
                                new Error(
                                  'The current value must be greater than to!',
                                ),
                              );
                            },
                          }),
                        ]}
                      >
                        <InputNumber
                          placeholder="to"
                          min={0}
                          precision={0}
                          className={styles.pageInputNumber}
                        />
                      </Form.Item>
                      {name > 0 && (
                        <MinusCircleOutlined onClick={() => remove(name)} />
                      )}
                    </Space>
                  ))}
                  <Form.Item>
                    <Button
                      type="dashed"
                      onClick={() => add()}
                      block
                      icon={<PlusOutlined />}
                    >
                      Add page
                    </Button>
                  </Form.Item>
                </>
              )}
            </Form.List>
          </>
        )}
        {showOne && (
          <Form.Item
            name={['parser_config', 'layout_recognize']}
            label="Layout recognize"
            initialValue={true}
            valuePropName="checked"
            tooltip={
              'Use visual models for layout analysis to better identify document structure, find where the titles, text blocks, images, and tables are. Without this feature, only the plain text of the PDF can be obtained.'
            }
          >
            <Switch />
          </Form.Item>
        )}
        {showPages && (
          <Form.Item
            noStyle
            dependencies={[['parser_config', 'layout_recognize']]}
          >
            {({ getFieldValue }) =>
              getFieldValue(['parser_config', 'layout_recognize']) && (
                <Form.Item
                  name={['parser_config', 'task_page_size']}
                  label="Task page size"
                  tooltip={`If using layout recognize, the PDF file will be split into groups of successive. Layout analysis will be performed parallelly between groups to increase the processing speed. 
                    The 'Task page size' determines the size of groups. The larger the page size is, the lower the chance of splitting continuous text between pages into different chunks.`}
                  initialValue={12}
                  rules={[
                    {
                      required: true,
                      message: 'Please input your task page size!',
                    },
                  ]}
                >
                  <InputNumber min={1} max={128} />
                </Form.Item>
              )
            }
          </Form.Item>
        )}
        {showMaxTokenNumber && <MaxTokenNumber></MaxTokenNumber>}
      </Form>
    </Modal>
  );
};
export default ChunkMethodModal;
