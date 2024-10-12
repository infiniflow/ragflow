import { useTranslate } from '@/hooks/common-hooks';
import { Form, Switch } from 'antd';

const ExcelToHtml = () => {
  const { t } = useTranslate('knowledgeDetails');
  return (
    <Form.Item
      name={['parser_config', 'html4excel']}
      label={t('html4excel')}
      initialValue={false}
      valuePropName="checked"
      tooltip={t('html4excelTip')}
    >
      <Switch />
    </Form.Item>
  );
};

export default ExcelToHtml;
