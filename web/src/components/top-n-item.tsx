import { useTranslate } from '@/hooks/common-hooks';
import { Form, Slider } from 'antd';

type FieldType = {
  top_n?: number;
};

interface IProps {
  initialValue?: number;
}

const TopNItem = ({ initialValue = 8 }: IProps) => {
  const { t } = useTranslate('chat');

  return (
    <Form.Item<FieldType>
      label={t('topN')}
      name={'top_n'}
      initialValue={initialValue}
      tooltip={t('topNTip')}
    >
      <Slider max={30} />
    </Form.Item>
  );
};

export default TopNItem;
