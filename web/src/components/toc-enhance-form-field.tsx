import { useTranslation } from 'react-i18next';
import { SwitchFormField } from './switch-fom-field';

type Props = { name: string };

export function TOCEnhanceFormField({ name }: Props) {
  const { t } = useTranslation();

  return (
    <SwitchFormField
      name={name}
      label={t('chat.tocEnhance')}
      tooltip={t('chat.tocEnhanceTip')}
    ></SwitchFormField>
  );
}
