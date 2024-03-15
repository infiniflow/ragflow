import { useSetModalState } from '@/hooks/commonHooks';
import {
  IApiKeySavingParams,
  ISystemModelSettingSavingParams,
  useFetchLlmList,
  useSaveApiKey,
  useSaveTenantInfo,
} from '@/hooks/llmHooks';
import { useOneNamespaceEffectsLoading } from '@/hooks/storeHooks';
import {
  useFetchTenantInfo,
  useSelectTenantInfo,
} from '@/hooks/userSettingHook';
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

      if (ret === 0) {
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

export const useSubmitSystemModelSetting = () => {
  const systemSetting = useSelectTenantInfo();
  const loading = useOneNamespaceEffectsLoading('settingModel', [
    'set_tenant_info',
  ]);
  const saveSystemModelSetting = useSaveTenantInfo();
  const {
    visible: systemSettingVisible,
    hideModal: hideSystemSettingModal,
    showModal: showSystemSettingModal,
  } = useSetModalState();

  const onSystemSettingSavingOk = useCallback(
    async (
      payload: Omit<ISystemModelSettingSavingParams, 'tenant_id' | 'name'>,
    ) => {
      const ret = await saveSystemModelSetting({
        tenant_id: systemSetting.tenant_id,
        name: systemSetting.name,
        ...payload,
      });

      if (ret === 0) {
        hideSystemSettingModal();
      }
    },
    [hideSystemSettingModal, saveSystemModelSetting, systemSetting],
  );

  return {
    saveSystemModelSettingLoading: loading,
    onSystemSettingSavingOk,
    systemSettingVisible,
    hideSystemSettingModal,
    showSystemSettingModal,
  };
};

export const useFetchSystemModelSettingOnMount = () => {
  const systemSetting = useSelectTenantInfo();
  useFetchLlmList();
  useFetchTenantInfo();

  return systemSetting;
};
