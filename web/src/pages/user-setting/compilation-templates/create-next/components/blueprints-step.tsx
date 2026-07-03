import { Button } from '@/components/ui/button';
import { UseFormReturn } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { FormSchemaType } from '@/pages/user-setting/compilation-templates/edit-template/schema';

type BlueprintsStepProps = {
  form: UseFormReturn<FormSchemaType>;
  onBack: () => void;
  onSave: () => void;
  isLoading: boolean;
};

export function BlueprintsStep({
  onBack,
  onSave,
  isLoading,
}: BlueprintsStepProps) {
  const { t } = useTranslation();

  return (
    <section className="flex-1 flex flex-col p-5">
      <div className="flex-1 flex flex-col items-center justify-center text-center space-y-4 max-w-md mx-auto">
        <h3 className="text-lg font-medium text-text-primary">
          {t('setting.blueprints')}
        </h3>
        <p className="text-sm text-text-secondary">
          {t('setting.blueprintsPlaceholder')}
        </p>
      </div>

      <footer className="shrink-0 px-5 py-4 border-t border-border-button flex items-center justify-between">
        <Button type="button" variant="outline" onClick={onBack}>
          {t('common.back')}
        </Button>
        <Button
          type="button"
          onClick={onSave}
          loading={isLoading}
          disabled={isLoading}
        >
          {t('common.save')}
        </Button>
      </footer>
    </section>
  );
}
