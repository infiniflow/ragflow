import { ButtonLoading } from '@/components/ui/button';
import { useSaveAgent } from '@/hooks/use-agent-request';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  AgentSettingId,
  SettingForm,
  SettingFormSchemaType,
} from './setting-form';

export default function AgentSetting() {
  const { t } = useTranslation();
  const { saveAgent, loading } = useSaveAgent();

  const submit = useCallback(
    async (values: SettingFormSchemaType) => {
      await saveAgent(values);
    },
    [saveAgent],
  );

  return (
    <section className="h-full overflow-y-auto p-5">
      <div className="mx-auto w-full max-w-3xl">
        <header className="pb-8">
          <h1 className="text-xl font-semibold text-text-primary">
            {t('flow.setting')}
          </h1>
        </header>
        <SettingForm submit={submit} />
        <footer className="mt-8 flex justify-end">
          <ButtonLoading type="submit" form={AgentSettingId} loading={loading}>
            {t('common.save')}
          </ButtonLoading>
        </footer>
      </div>
    </section>
  );
}
