import { useTranslate } from '@/hooks/common-hooks';
import { Form, Slider } from 'antd';

type FieldType = {
  top_n?: number;
};

interface IProps {
  initialValue?: number;
  max?: number;
}

const TopNItem = ({ initialValue = 8, max = 30 }: IProps) => {
  const { t } = useTranslate('chat');

  return (
    <Form.Item<FieldType>
      label={t('topN')}
      name={'top_n'}
      initialValue={initialValue}
      tooltip={t('topNTip')}
    >
      <Slider max={max} />
    </Form.Item>
  );
};

export default TopNItem;
