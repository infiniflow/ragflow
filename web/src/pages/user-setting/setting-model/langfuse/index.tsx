import SvgIcon from '@/components/svg-icon';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { useFetchLangfuseConfig } from '@/hooks/use-user-setting-request';
import { Eye, Settings2 } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { LangfuseConfigurationDialog } from './langfuse-configuration-dialog';
import { useSaveLangfuseConfiguration } from './use-save-langfuse-configuration';

export function LangfuseCard() {
  const {
    saveLangfuseConfigurationOk,
    showSaveLangfuseConfigurationModal,
    hideSaveLangfuseConfigurationModal,
    saveLangfuseConfigurationVisible,
    loading,
  } = useSaveLangfuseConfiguration();
  const { t } = useTranslation();
  const { data } = useFetchLangfuseConfig();

  const handleView = useCallback(() => {
    window.open(
      `https://cloud.langfuse.com/project/${data?.project_id}`,
      '_blank',
    );
  }, [data?.project_id]);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex justify-between">
          <div className="flex items-center gap-4">
            <SvgIcon name={'langfuse'} width={24} height={24}></SvgIcon>
            Langfuse
          </div>
          <div className="flex gap-4 items-center">
            {data && (
              <Button variant={'outline'} size={'sm'} onClick={handleView}>
                <Eye /> {t('setting.view')}
              </Button>
            )}
            <Button size={'sm'} onClick={showSaveLangfuseConfigurationModal}>
              <Settings2 />
              {t('setting.configuration')}
            </Button>
          </div>
        </CardTitle>
        <CardDescription>{t('setting.langfuseDescription')}</CardDescription>
      </CardHeader>
      {saveLangfuseConfigurationVisible && (
        <LangfuseConfigurationDialog
          hideModal={hideSaveLangfuseConfigurationModal}
          onOk={saveLangfuseConfigurationOk}
          loading={loading}
        ></LangfuseConfigurationDialog>
      )}
    </Card>
  );
}
