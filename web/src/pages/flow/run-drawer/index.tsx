import { Authorization } from '@/constants/authorization';
import { useSetModalState } from '@/hooks/common-hooks';
import { useFetchFlow } from '@/hooks/flow-hooks';
import { useSetSelectedRecord } from '@/hooks/logic-hooks';
import { IModalProps } from '@/interfaces/common';
import api from '@/utils/api';
import { getAuthorization } from '@/utils/authorization-util';
import { InboxOutlined } from '@ant-design/icons';
import {
  Button,
  Drawer,
  Flex,
  Form,
  FormItemProps,
  Input,
  InputNumber,
  Select,
  Switch,
  Upload,
} from 'antd';
import { pick } from 'lodash';
import { Link2, Trash2 } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { BeginQueryType } from '../constant';
import {
  useGetBeginNodeDataQuery,
  useSaveGraphBeforeOpeningDebugDrawer,
} from '../hooks';
import { BeginQuery } from '../interface';
import useGraphStore from '../store';
import { getDrawerWidth } from '../utils';
import { PopoverForm } from './popover-form';

const RunDrawer = ({
  hideModal,
  showModal: showChatModal,
}: IModalProps<any>) => {
  const { t } = useTranslation();
  const { data } = useFetchFlow();
  const [form] = Form.useForm();
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);
  const {
    visible,
    hideModal: hidePopover,
    switchVisible,
    showModal: showPopover,
  } = useSetModalState();
  const { setRecord, currentRecord } = useSetSelectedRecord<number>();

  const handleShowPopover = useCallback(
    (idx: number) => () => {
      setRecord(idx);
      showPopover();
    },
    [setRecord, showPopover],
  );

  const handleRemoveUrl = useCallback(
    (key: number, index: number) => () => {
      const list: any[] = form.getFieldValue(key);

      form.setFieldValue(
        key,
        list.filter((_, idx) => idx !== index),
      );
    },
    [form],
  );

  const getBeginNodeDataQuery = useGetBeginNodeDataQuery();
  const query: BeginQuery[] = getBeginNodeDataQuery();

  const normFile = (e: any) => {
    if (Array.isArray(e)) {
      return e;
    }
    return e?.fileList;
  };

  const renderWidget = useCallback(
    (q: BeginQuery, idx: number) => {
      const props: FormItemProps & { key: number } = {
        key: idx,
        label: q.name,
        name: idx,
      };
      if (q.optional === false) {
        props.rules = [{ required: true }];
      }

      const BeginQueryTypeMap = {
        [BeginQueryType.Line]: (
          <Form.Item {...props}>
            <Input></Input>
          </Form.Item>
        ),
        [BeginQueryType.Paragraph]: (
          <Form.Item {...props}>
            <Input.TextArea rows={4}></Input.TextArea>
          </Form.Item>
        ),
        [BeginQueryType.Options]: (
          <Form.Item {...props}>
            <Select
              allowClear
              options={q.options?.map((x) => ({ label: x, value: x })) ?? []}
            ></Select>
          </Form.Item>
        ),
        [BeginQueryType.File]: (
          <Form.Item
            {...props}
            valuePropName="fileList"
            getValueFromEvent={normFile}
          >
            <Upload.Dragger
              name="file"
              action={api.parse}
              multiple
              headers={{ [Authorization]: getAuthorization() }}
            >
              <p className="ant-upload-drag-icon">
                <InboxOutlined />
              </p>
              <p className="ant-upload-text">
                Click or drag file to this area to upload
              </p>
              <p className="ant-upload-hint">
                Support for a single or bulk upload.
              </p>
            </Upload.Dragger>
          </Form.Item>
        ),
        [BeginQueryType.Integer]: (
          <Form.Item {...props}>
            <InputNumber></InputNumber>
          </Form.Item>
        ),
        [BeginQueryType.Boolean]: (
          <Form.Item valuePropName={'checked'} {...props}>
            <Switch></Switch>
          </Form.Item>
        ),
        [BeginQueryType.Url]: (
          <>
            <Form.Item {...pick(props, ['key', 'label'])}>
              <PopoverForm visible={visible} switchVisible={switchVisible}>
                <Button onClick={handleShowPopover(idx)}>
                  paste file link
                </Button>
              </PopoverForm>
            </Form.Item>
            <Form.Item name={idx} noStyle />
            <Form.Item
              noStyle
              shouldUpdate={(prevValues, curValues) =>
                prevValues[idx] !== curValues[idx]
              }
            >
              {({ getFieldValue }) => {
                const urlInfo: { url: string; result: string }[] =
                  getFieldValue(idx) || [];
                return urlInfo.length ? (
                  <Flex vertical gap={8}>
                    {urlInfo.map((u, index) => (
                      <div
                        key={index}
                        className="flex items-center justify-between gap-2 hover:bg-slate-100 group"
                      >
                        <Link2 className="size-5"></Link2>
                        <span className="flex-1 truncate"> {u.url}</span>
                        <Trash2
                          className="size-4 invisible group-hover:visible cursor-pointer"
                          onClick={handleRemoveUrl(idx, index)}
                        />
                      </div>
                    ))}
                  </Flex>
                ) : null;
              }}
            </Form.Item>
          </>
        ),
      };

      return BeginQueryTypeMap[q.type as BeginQueryType];
    },
    [handleRemoveUrl, handleShowPopover, switchVisible, visible],
  );

  const { handleRun } = useSaveGraphBeforeOpeningDebugDrawer(showChatModal!);

  const handleRunAgent = useCallback(
    (nextValues: Record<string, any>) => {
      const currentNodes = updateNodeForm('begin', nextValues, ['query']);
      handleRun(currentNodes);
      hideModal?.();
    },
    [handleRun, hideModal, updateNodeForm],
  );

  const onOk = useCallback(async () => {
    const values = await form.validateFields();
    const nextValues = Object.entries(values).map(([key, value]) => {
      const item = query[Number(key)];
      let nextValue = value;
      if (Array.isArray(value)) {
        nextValue = ``;

        value.forEach((x, idx) => {
          if (x?.originFileObj instanceof File) {
            if (idx === 0) {
              nextValue += `${x.name}\n\n${x.response.data}\n\n`;
            } else {
              nextValue += `${x.response.data}\n\n`;
            }
          } else {
            if (idx === 0) {
              nextValue += `${x.url}\n\n${x.result}\n\n`;
            } else {
              nextValue += `${x.result}\n\n`;
            }
          }
        });
      }
      return { ...item, value: nextValue };
    });
    handleRunAgent(nextValues);
  }, [form, handleRunAgent, query]);

  return (
    <Drawer
      title={data.title}
      placement="right"
      onClose={hideModal}
      open
      getContainer={false}
      width={getDrawerWidth()}
      mask={false}
    >
      <Form.Provider
        onFormFinish={(name, { values, forms }) => {
          if (name === 'urlForm') {
            const { basicForm } = forms;
            const urlInfo = basicForm.getFieldValue(currentRecord) || [];
            basicForm.setFieldsValue({ [currentRecord]: [...urlInfo, values] });
            hidePopover();
          }
        }}
      >
        <Form
          name="basicForm"
          autoComplete="off"
          layout={'vertical'}
          form={form}
        >
          {query.map((x, idx) => {
            return renderWidget(x, idx);
          })}
        </Form>
      </Form.Provider>
      <Button type={'primary'} block onClick={onOk}>
        {t('flow.run')}
      </Button>
    </Drawer>
  );
};

export default RunDrawer;
