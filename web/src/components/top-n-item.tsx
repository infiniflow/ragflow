import { useTranslate } from '@/hooks/commonHooks';
import { Form, Slider } from 'antd';

type FieldType = {
  top_n?: number;
};

const TopNItem = () => {
  const { t } = useTranslate('chat');

  return (
    <Form.Item<FieldType>
      label={t('topN')}
      name={'top_n'}
      initialValue={8}
      tooltip={t('topNTip')}
    >
      <Slider max={30} />
    </Form.Item>
  );
};

export default TopNItem;
