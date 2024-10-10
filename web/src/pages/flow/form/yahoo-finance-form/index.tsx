import { useTranslate } from '@/hooks/common-hooks';
import { Form, Switch } from 'antd';
import { IOperatorForm } from '../../interface';

const YahooFinanceForm = ({ onValuesChange, form }: IOperatorForm) => {
  const { t } = useTranslate('flow');

  return (
    <Form
      name="basic"
      labelCol={{ span: 10 }}
      wrapperCol={{ span: 14 }}
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
    >
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
