import { useTranslate } from '@/hooks/common-hooks';
import { Form, Select } from 'antd';
import { useMemo } from 'react';
import { FileSource } from '../../constant';
import { IOperatorForm } from '../../interface';
import DynamicInputVariable from '../components/dynamic-input-variable';

const FileReaderForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslate('flow');
  const fileSource = useMemo(() => {
    return FileSource.map((x) => ({
      value: x,
      label: t(`fileSource.${x}`),
    }));
  }, [t]);

  return (
    <Form
      name="basic"
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
      layout={'vertical'}
    >
      <Form.Item
        name={['type']}
        label={t('fileSource.type')}
        initialValue={t('fileSource.local')}
      >
        <Select options={fileSource}></Select>
      </Form.Item>
      <DynamicInputVariable node={node}></DynamicInputVariable>
    </Form>
  );
};

export default FileReaderForm;
