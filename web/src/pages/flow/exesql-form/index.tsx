import TopNItem from '@/components/top-n-item';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Input, InputNumber, Select } from 'antd';
import { ExeSQLOptions } from '../constant';
import { IOperatorForm } from '../interface';

const ExeSQLForm = ({ onValuesChange, form }: IOperatorForm) => {
  const { t } = useTranslate('flow');

  return (
    <Form
      name="basic"
      labelCol={{ span: 7 }}
      wrapperCol={{ span: 17 }}
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
    >
      <Form.Item
        label={t('dbType')}
        name={'db_type'}
        rules={[{ required: true }]}
      >
        <Select options={ExeSQLOptions}></Select>
      </Form.Item>
      <Form.Item
        label={t('database')}
        name={'database'}
        rules={[{ required: true }]}
      >
        <Input></Input>
      </Form.Item>
      <Form.Item
        label={t('username')}
        name={'username'}
        rules={[{ required: true }]}
      >
        <Input></Input>
      </Form.Item>
      <Form.Item label={t('host')} name={'host'} rules={[{ required: true }]}>
        <Input></Input>
      </Form.Item>
      <Form.Item label={t('port')} name={'port'} rules={[{ required: true }]}>
        <InputNumber></InputNumber>
      </Form.Item>
      <Form.Item
        label={t('password')}
        name={'password'}
        rules={[{ required: true }]}
      >
        <Input.Password></Input.Password>
      </Form.Item>
      <Form.Item
        label={t('loop')}
        name={'loop'}
        tooltip={t('loopTip')}
        rules={[{ required: true }]}
      >
        <InputNumber></InputNumber>
      </Form.Item>
      <TopNItem initialValue={30} max={1000}></TopNItem>
    </Form>
  );
};

export default ExeSQLForm;
