import { useTranslate } from '@/hooks/common-hooks';
import { Form, Switch } from 'antd';
import { IOperatorForm } from '../../interface';
import DynamicInputVariable from '../components/dynamic-input-variable';

const YahooFinanceForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslate('flow');

  return (
    <Form
      name="basic"
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
      layout={'vertical'}
    >
      <DynamicInputVariable node={node}></DynamicInputVariable>
      <Form.Item label={t('info')} name={'info'}>
        <Switch></Switch>
      </Form.Item>
      <Form.Item label={t('history')} name={'history'}>
        <Switch></Switch>
      </Form.Item>
      <Form.Item label={t('financials')} name={'financials'}>
        <Switch></Switch>
      </Form.Item>
      <Form.Item label={t('balanceSheet')} name={'balance_sheet'}>
        <Switch></Switch>
      </Form.Item>
      <Form.Item label={t('cashFlowStatement')} name={'cash_flow_statement'}>
        <Switch></Switch>
      </Form.Item>
      <Form.Item label={t('news')} name={'news'}>
        <Switch></Switch>
      </Form.Item>
    </Form>
  );
};

export default YahooFinanceForm;
