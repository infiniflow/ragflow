import { ButtonLoading } from '@/components/ui/button';
import { useTranslation } from 'react-i18next';

type SaveButtonProps = {
  loading: boolean;
};

export function SavingButton({ loading }: SaveButtonProps) {
  const { t } = useTranslation();

  return (
    <ButtonLoading
      type="submit"
      loading={loading}
      data-testid="chat-settings-save"
    >
      {t('common.save')}
    </ButtonLoading>
  );
}
