import { Form, Input } from 'antd';
import { useTranslation } from 'react-i18next';

interface IProps {
  value?: string | undefined;
  onChange?: (val: string | undefined) => void;
}

const DelimiterInput = ({ value, onChange }: IProps) => {
  const nextValue = value?.replaceAll('\n', '\\n');
  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value;
    const nextValue = val.replaceAll('\\n', '\n');
    onChange?.(nextValue);
  };
  return <Input value={nextValue} onChange={handleInputChange}></Input>;
};

const Delimiter = () => {
  const { t } = useTranslation();

  return (
    <Form.Item
      name={['parser_config', 'delimiter']}
      label={t('knowledgeDetails.delimiter')}
      initialValue={`\\n!?;。；！？`}
      rules={[{ required: true }]}
    >
      <DelimiterInput />
    </Form.Item>
  );
};

export default Delimiter;
