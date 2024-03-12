import { useSetModalState } from '@/hooks/commonHooks';
import { IApiKeySavingParams, useSaveApiKey } from '@/hooks/llmHooks';
import { useOneNamespaceEffectsLoading } from '@/hooks/storeHooks';
import { useCallback, useState } from 'react';

type SavingParamsState = Omit<IApiKeySavingParams, 'api_key'>;

export const useSubmitApiKey = () => {
  const [savingParams, setSavingParams] = useState<SavingParamsState>(
    {} as SavingParamsState,
  );
  const saveApiKey = useSaveApiKey();
  const {
    visible: apiKeyVisible,
    hideModal: hideApiKeyModal,
    showModal: showApiKeyModal,
  } = useSetModalState();

  const onApiKeySavingOk = useCallback(
    async (apiKey: string) => {
      const ret = await saveApiKey({ ...savingParams, api_key: apiKey });

      if (ret.retcode === 0) {
        hideApiKeyModal();
      }
    },
    [hideApiKeyModal, saveApiKey, savingParams],
  );

  const onShowApiKeyModal = useCallback(
    (savingParams: SavingParamsState) => {
      setSavingParams(savingParams);
      showApiKeyModal();
    },
    [showApiKeyModal, setSavingParams],
  );

  const loading = useOneNamespaceEffectsLoading('settingModel', [
    'set_api_key',
  ]);

  return {
    saveApiKeyLoading: loading,
    initialApiKey: '',
    onApiKeySavingOk,
    apiKeyVisible,
    hideApiKeyModal,
    showApiKeyModal: onShowApiKeyModal,
  };
};
