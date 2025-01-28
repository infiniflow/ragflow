import { Form, Switch } from 'antd';
import { useTranslation } from 'react-i18next';

type IProps = {
  filedName: string[];
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
