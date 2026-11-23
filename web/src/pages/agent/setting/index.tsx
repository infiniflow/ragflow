import { PageFrame } from '@/components/page-frame';
import { SettingsFormContainer } from '@/components/settings-form-container';
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
    <PageFrame className="overflow-y-auto">
      <SettingsFormContainer className="p-5">
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
      </SettingsFormContainer>
    </PageFrame>
  );
}
