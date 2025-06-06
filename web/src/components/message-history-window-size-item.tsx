import { Form, InputNumber } from 'antd';
import { useMemo } from 'react';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from './ui/form';
import { BlurInput, Input } from './ui/input';

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

type MessageHistoryWindowSizeFormFieldProps = {
  useBlurInput?: boolean;
};

export function MessageHistoryWindowSizeFormField({
  useBlurInput = false,
}: MessageHistoryWindowSizeFormFieldProps) {
  const form = useFormContext();
  const { t } = useTranslation();

  const NextInput = useMemo(() => {
    return useBlurInput ? BlurInput : Input;
  }, [useBlurInput]);

  return (
    <FormField
      control={form.control}
      name={'message_history_window_size'}
      render={({ field }) => (
        <FormItem>
          <FormLabel tooltip={t('flow.messageHistoryWindowSizeTip')}>
            {t('flow.messageHistoryWindowSize')}
          </FormLabel>
          <FormControl>
            <NextInput {...field} type={'number'}></NextInput>
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
