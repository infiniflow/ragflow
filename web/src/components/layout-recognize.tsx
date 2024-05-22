import { useTranslate } from '@/hooks/commonHooks';
import { Form, Switch } from 'antd';

const LayoutRecognize = () => {
  const { t } = useTranslate('knowledgeDetails');
  return (
    <Form.Item
      name={['parser_config', 'layout_recognize']}
      label={t('layoutRecognize')}
      initialValue={true}
      valuePropName="checked"
      tooltip={t('layoutRecognizeTip')}
    >
      <Switch />
    </Form.Item>
  );
};

export default LayoutRecognize;
