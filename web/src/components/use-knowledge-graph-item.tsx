import { useTranslation } from 'react-i18next';
import { SwitchFormField } from './switch-fom-field';

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
