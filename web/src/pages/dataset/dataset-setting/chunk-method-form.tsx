import { Button } from '@/components/ui/button';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { NaiveConfiguration } from './naive';
import { SavingButton } from './saving-button';

export function ChunkMethodForm() {
  const form = useFormContext();
  const { t } = useTranslation();

  return (
    <section className="h-full flex flex-col">
      <div className="overflow-auto flex-1 min-h-0">
        <NaiveConfiguration></NaiveConfiguration>
      </div>
      <div className="text-right pt-4 flex justify-end gap-3">
        <Button
          type="reset"
          className="bg-transparent text-color-white hover:bg-transparent border-gray-500 border-[1px]"
          onClick={() => {
            form.reset();
          }}
        >
          {t('knowledgeConfiguration.cancel')}
        </Button>
        <SavingButton></SavingButton>
      </div>
    </section>
  );
}
