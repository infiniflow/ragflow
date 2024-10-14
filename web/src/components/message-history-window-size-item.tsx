import { Form, InputNumber } from 'antd';
import { useTranslation } from 'react-i18next';

const MessageHistoryWindowSizeItem = ({
  initialValue,
}: {
  initialValue: number;
}) => {
  const { t } = useTranslation();

  return (
    <Form.Item
      name={'message_history_window_size'}
      label={t('flow.messageHistoryWindowSize')}
      initialValue={initialValue}
      tooltip={t('flow.messageHistoryWindowSizeTip')}
    >
      <InputNumber style={{ width: '100%' }} />
    </Form.Item>
  );
};

export default MessageHistoryWindowSizeItem;
