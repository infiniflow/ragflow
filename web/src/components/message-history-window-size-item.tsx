import { Form, InputNumber } from 'antd';
import { useTranslation } from 'react-i18next';

const MessageHistoryWindowSizeItem = ({
  initialValue,
}: {
  initialValue: number;
}) => {
  const { t } = useTranslation('flow');

  return (
    <Form.Item
      name={'message_history_window_size'}
      label={t('messageHistoryWindowSize')}
      initialValue={initialValue}
      tooltip={t('messageHistoryWindowSizeTip')}
    >
      <InputNumber style={{ width: '100%' }} />
    </Form.Item>
  );
};

export default MessageHistoryWindowSizeItem;
