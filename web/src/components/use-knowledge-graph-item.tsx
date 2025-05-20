import { Form, Switch } from 'antd';
import { useTranslation } from 'react-i18next';
import { SwitchFormField } from './switch-fom-field';

type IProps = {
  filedName: string[] | string;
};

export function UseKnowledgeGraphItem({ filedName }: IProps) {
  const { t } = useTranslation();

  return (
    <Form.Item
      label={t('chat.useKnowledgeGraph')}
      tooltip={t('chat.useKnowledgeGraphTip')}
      name={filedName}
      initialValue={false}
    >
      <Switch></Switch>
    </Form.Item>
  );
}

interface UseKnowledgeGraphFormFieldProps {
  name: string;
}

export function UseKnowledgeGraphFormField({
  name,
}: UseKnowledgeGraphFormFieldProps) {
  const { t } = useTranslation();

  return (
    <SwitchFormField
      name={name}
      label={t('chat.useKnowledgeGraph')}
      tooltip={t('chat.useKnowledgeGraphTip')}
    ></SwitchFormField>
  );
}
