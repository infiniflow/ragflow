import { Authorization } from '@/constants/authorization';
import { useSetModalState } from '@/hooks/common-hooks';
import { useSetSelectedRecord } from '@/hooks/logic-hooks';
import { useHandleSubmittable } from '@/hooks/login-hooks';
import api from '@/utils/api';
import { getAuthorization } from '@/utils/authorization-util';
import { UploadOutlined } from '@ant-design/icons';
import {
  Button,
  Form,
  FormItemProps,
  Input,
  InputNumber,
  Select,
  Switch,
  Upload,
} from 'antd';
import { UploadChangeParam, UploadFile } from 'antd/es/upload';
import { pick } from 'lodash';
import { Link } from 'lucide-react';
import React, { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { BeginQueryType } from '../constant';
import { useGetBeginNodeDataQuery } from '../hooks';
import { BeginQuery } from '../interface';
import useGraphStore from '../store';
import { PopoverForm } from './popover-form';

import styles from './index.less';

interface IProps {
  parameters: BeginQuery[];
}

const DebugContent = ({ parameters }: IProps) => {
  const { t } = useTranslation();
  const [form] = Form.useForm();
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);
  const {
    visible,
    hideModal: hidePopover,
    switchVisible,
    showModal: showPopover,
  } = useSetModalState();
  const { setRecord, currentRecord } = useSetSelectedRecord<number>();
  const { submittable } = useHandleSubmittable(form);
  const [isUploading, setIsUploading] = useState(false);

  const handleShowPopover = useCallback(
    (idx: number) => () => {
      setRecord(idx);
      showPopover();
    },
    [setRecord, showPopover],
  );

  const getBeginNodeDataQuery = useGetBeginNodeDataQuery();

  const normFile = (e: any) => {
    if (Array.isArray(e)) {
      return e;
    }
    return e?.fileList;
  };

  const onChange = useCallback(
    (optional: boolean) =>
      ({ fileList }: UploadChangeParam<UploadFile>) => {
        if (!optional) {
          setIsUploading(fileList.some((x) => x.status === 'uploading'));
        }
      },
    [],
  );

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

      const urlList: { url: string; result: string }[] =
        form.getFieldValue(idx) || [];

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
          <React.Fragment key={idx}>
            <Form.Item label={q.name} required={!q.optional}>
              <div className="relative">
                <Form.Item
                  {...props}
                  valuePropName="fileList"
                  getValueFromEvent={normFile}
                  noStyle
                >
                  <Upload
                    name="file"
                    action={api.parse}
                    multiple
                    headers={{ [Authorization]: getAuthorization() }}
                    onChange={onChange(q.optional)}
                  >
                    <Button icon={<UploadOutlined />}>
                      {t('common.upload')}
                    </Button>
                  </Upload>
                </Form.Item>
                <Form.Item
                  {...pick(props, ['key', 'label', 'rules'])}
                  required={!q.optional}
                  className={urlList.length > 0 ? 'mb-1' : ''}
                  noStyle
                >
                  <PopoverForm visible={visible} switchVisible={switchVisible}>
                    <Button
                      onClick={handleShowPopover(idx)}
                      className="absolute left-1/2 top-0"
                      icon={<Link className="size-3" />}
                    >
                      {t('flow.pasteFileLink')}
                    </Button>
                  </PopoverForm>
                </Form.Item>
              </div>
            </Form.Item>
            <Form.Item name={idx} noStyle {...pick(props, ['rules'])} />
          </React.Fragment>
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
      };

      return (
        BeginQueryTypeMap[q.type as BeginQueryType] ??
        BeginQueryTypeMap[BeginQueryType.Line]
      );
    },
    [form, handleShowPopover, onChange, switchVisible, t, visible],
  );

  // const { handleRun } = useSaveGraphBeforeOpeningDebugDrawer(showChatModal!);

  // const handleRunAgent = useCallback(
  //   (nextValues: Record<string, any>) => {
  //     const currentNodes = updateNodeForm('begin', nextValues, ['query']);
  //     handleRun(currentNodes);
  //     hideModal?.();
  //   },
  //   [handleRun, hideModal, updateNodeForm],
  // );

  const onOk = useCallback(async () => {
    const values = await form.validateFields();
    const nextValues = Object.entries(values).map(([key, value]) => {
      const item = parameters[Number(key)];
      let nextValue = value;
      if (Array.isArray(value)) {
        nextValue = ``;

        value.forEach((x) => {
          nextValue +=
            x?.originFileObj instanceof File
              ? `${x.name}\n${x.response?.data}\n----\n`
              : `${x.url}\n${x.result}\n----\n`;
        });
      }
      return { ...item, value: nextValue };
    });

    console.log('🚀 ~ nextValues ~ nextValues:', nextValues);

    // handleRunAgent(nextValues);
  }, [form, parameters]);

  return (
    <>
      <section className={styles.formWrapper}>
        <Form.Provider
          onFormFinish={(name, { values, forms }) => {
            if (name === 'urlForm') {
              const { basicForm } = forms;
              const urlInfo = basicForm.getFieldValue(currentRecord) || [];
              basicForm.setFieldsValue({
                [currentRecord]: [...urlInfo, { ...values, name: values.url }],
              });
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
            {parameters.map((x, idx) => {
              return renderWidget(x, idx);
            })}
          </Form>
        </Form.Provider>
      </section>
      <Button
        type={'primary'}
        block
        onClick={onOk}
        disabled={!submittable || isUploading}
      >
        {t('common.next')}
      </Button>
    </>
  );
};

export default DebugContent;
