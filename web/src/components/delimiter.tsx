import { Form, Input } from 'antd';
import { useTranslation } from 'react-i18next';

interface IProps {
  value?: string | undefined;
  onChange?: (val: string | undefined) => void;
  maxLength?: number;
}

export const DelimiterInput = ({ value, onChange, maxLength }: IProps) => {
  const nextValue = value?.replaceAll('\n', '\\n');
  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value;
    const nextValue = val.replaceAll('\\n', '\n');
    onChange?.(nextValue);
  };
  return (
    <Input
      value={nextValue}
      onChange={handleInputChange}
      maxLength={maxLength}
    ></Input>
  );
};

const Delimiter = () => {
  const { t } = useTranslation();

  return (
    <Form.Item
      name={['parser_config', 'delimiter']}
      label={t('knowledgeDetails.delimiter')}
      initialValue={`\n`}
      rules={[{ required: true }]}
      tooltip={t('knowledgeDetails.delimiterTip')}
    >
      <DelimiterInput />
    </Form.Item>
  );
};

export default Delimiter;
