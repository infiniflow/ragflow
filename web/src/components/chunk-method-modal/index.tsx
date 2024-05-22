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
  Select,
  Space,
  Tooltip,
} from 'antd';
import omit from 'lodash/omit';
import React, { useEffect, useMemo } from 'react';
import { useFetchParserListOnMount } from './hooks';

import { useTranslate } from '@/hooks/commonHooks';
import LayoutRecognize from '../layout-recognize';
import styles from './index.less';

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
  loading,
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
      confirmLoading={loading}
    >
      <Space size={[0, 8]} wrap>
        <Form.Item label={t('chunkMethod')} className={styles.chunkMethod}>
          <Select
            style={{ width: 120 }}
            onChange={handleChange}
            value={selectedTag}
            options={parserList}
          />
        </Form.Item>
      </Space>
      {hideDivider || <Divider></Divider>}
      <Form name="dynamic_form_nest_item" autoComplete="off" form={form}>
        {showPages && (
          <>
            <Space>
              <p>{t('pageRanges')}:</p>
              <Tooltip title={t('pageRangesTip')}>
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
                            message: t('fromMessage'),
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
                                new Error(t('greaterThanPrevious')),
                              );
                            },
                          }),
                        ]}
                      >
                        <InputNumber
                          placeholder={t('fromPlaceholder')}
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
                            message: t('toMessage'),
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
                                new Error(t('greaterThan')),
                              );
                            },
                          }),
                        ]}
                      >
                        <InputNumber
                          placeholder={t('toPlaceholder')}
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
                      {t('addPage')}
                    </Button>
                  </Form.Item>
                </>
              )}
            </Form.List>
          </>
        )}
        {showOne && <LayoutRecognize></LayoutRecognize>}
        {showPages && (
          <Form.Item
            noStyle
            dependencies={[['parser_config', 'layout_recognize']]}
          >
            {({ getFieldValue }) =>
              getFieldValue(['parser_config', 'layout_recognize']) && (
                <Form.Item
                  name={['parser_config', 'task_page_size']}
                  label={t('taskPageSize')}
                  tooltip={t('taskPageSizeTip')}
                  initialValue={12}
                  rules={[
                    {
                      required: true,
                      message: t('taskPageSizeMessage'),
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
