import LLMSelect from '@/components/llm-select';
import TopNItem from '@/components/top-n-item';
import { useTranslate } from '@/hooks/common-hooks';
import { useTestDbConnect } from '@/hooks/flow-hooks';
import { Button, Flex, Form, Input, InputNumber, Select } from 'antd';
import { useCallback } from 'react';
import { ExeSQLOptions } from '../../constant';
import { IOperatorForm } from '../../interface';
import DynamicInputVariable from '../components/dynamic-input-variable';

const ExeSQLForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslate('flow');
  const { testDbConnect, loading } = useTestDbConnect();

  const handleTest = useCallback(async () => {
    const ret = await form?.validateFields();
    testDbConnect(ret);
  }, [form, testDbConnect]);

  return (
    <Form
      name="basic"
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
      layout={'vertical'}
    >
      <DynamicInputVariable node={node}></DynamicInputVariable>
      <Form.Item
        name={'llm_id'}
        label={t('model', { keyPrefix: 'chat' })}
        tooltip={t('modelTip', { keyPrefix: 'chat' })}
      >
        <LLMSelect></LLMSelect>
      </Form.Item>
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
      <Flex justify={'end'}>
        <Button type={'primary'} loading={loading} onClick={handleTest}>
          Test
        </Button>
      </Flex>
    </Form>
  );
};

export default ExeSQLForm;
