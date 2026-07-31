import { useSetModalState } from '@/hooks/common-hooks';
import { useSetLangfuseConfig } from '@/hooks/use-user-setting-request';
import { ISetLangfuseConfigRequestBody } from '@/interfaces/request/system';
import { useCallback } from 'react';

export const useSaveLangfuseConfiguration = () => {
  const {
    visible: saveLangfuseConfigurationVisible,
    hideModal: hideSaveLangfuseConfigurationModal,
    showModal: showSaveLangfuseConfigurationModal,
  } = useSetModalState();
  const { setLangfuseConfig, loading } = useSetLangfuseConfig();

  const onSaveLangfuseConfigurationOk = useCallback(
    async (params: ISetLangfuseConfigRequestBody) => {
      const ret = await setLangfuseConfig(params);

      if (ret === 0) {
        hideSaveLangfuseConfigurationModal();
      }
      return ret;
    },
    [hideSaveLangfuseConfigurationModal],
  );

  return {
    loading,
    saveLangfuseConfigurationOk: onSaveLangfuseConfigurationOk,
    saveLangfuseConfigurationVisible,
    hideSaveLangfuseConfigurationModal,
    showSaveLangfuseConfigurationModal,
  };
};
