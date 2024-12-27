import { useResetFormOnCloseModal } from '@/hooks/logic-hooks';
import { IModalProps } from '@/interfaces/common';
import { Form, Input, Modal, Select, Switch } from 'antd';
import { DefaultOptionType } from 'antd/es/select';
import { useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { BeginQueryType, BeginQueryTypeIconMap } from '../../constant';
import { BeginQuery } from '../../interface';
import BeginDynamicOptions from './begin-dynamic-options';

export const ModalForm = ({
  visible,
  initialValue,
  hideModal,
  otherThanCurrentQuery,
}: IModalProps<BeginQuery> & {
  initialValue: BeginQuery;
  otherThanCurrentQuery: BeginQuery[];
}) => {
  const { t } = useTranslation();
  const [form] = Form.useForm();
  const options = useMemo(() => {
    return Object.values(BeginQueryType).reduce<DefaultOptionType[]>(
      (pre, cur) => {
        const Icon = BeginQueryTypeIconMap[cur];

        return [
          ...pre,
          {
            label: (
              <div className="flex items-center gap-2">
                <Icon
                  className={`size-${cur === BeginQueryType.Options ? 4 : 5}`}
                ></Icon>
                {cur}
              </div>
            ),
            value: cur,
          },
        ];
      },
      [],
    );
  }, []);

  useResetFormOnCloseModal({
    form,
    visible: visible,
  });

  useEffect(() => {
    form.setFieldsValue(initialValue);
  }, [form, initialValue]);

  const onOk = () => {
    form.submit();
  };

  return (
    <Modal
      title={t('flow.variableSettings')}
      open={visible}
      onOk={onOk}
      onCancel={hideModal}
      centered
    >
      <Form form={form} layout="vertical" name="queryForm" autoComplete="false">
        <Form.Item
          name="type"
          label="Type"
          rules={[{ required: true }]}
          initialValue={BeginQueryType.Line}
        >
          <Select options={options} />
        </Form.Item>
        <Form.Item
          name="key"
          label="Key"
          rules={[
            { required: true },
            () => ({
              validator(_, value) {
                if (
                  !value ||
                  !otherThanCurrentQuery.some((x) => x.key === value)
                ) {
                  return Promise.resolve();
                }
                return Promise.reject(new Error('The key cannot be repeated!'));
              },
            }),
          ]}
        >
          <Input />
        </Form.Item>
        <Form.Item name="name" label="Name" rules={[{ required: true }]}>
          <Input />
        </Form.Item>
        <Form.Item
          name="optional"
          label={'Optional'}
          valuePropName="checked"
          initialValue={false}
        >
          <Switch />
        </Form.Item>
        <Form.Item
          shouldUpdate={(prevValues, curValues) =>
            prevValues.type !== curValues.type
          }
        >
          {({ getFieldValue }) => {
            const type: BeginQueryType = getFieldValue('type');
            return (
              type === BeginQueryType.Options && (
                <BeginDynamicOptions></BeginDynamicOptions>
              )
            );
          }}
        </Form.Item>
      </Form>
    </Modal>
  );
};
