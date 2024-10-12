import { useSetModalState, useShowDeleteConfirm } from '@/hooks/common-hooks';
import {
  IApiKeySavingParams,
  ISystemModelSettingSavingParams,
  useAddLlm,
  useDeleteFactory,
  useDeleteLlm,
  useSaveApiKey,
  useSaveTenantInfo,
  useSelectLlmOptionsByModelType,
} from '@/hooks/llm-hooks';
import { useFetchTenantInfo } from '@/hooks/user-setting-hooks';
import { IAddLlmRequestBody } from '@/interfaces/request/llm';
import { useCallback, useState } from 'react';
import { ApiKeyPostBody } from '../interface';

type SavingParamsState = Omit<IApiKeySavingParams, 'api_key'>;

export const useSubmitApiKey = () => {
  const [savingParams, setSavingParams] = useState<SavingParamsState>(
    {} as SavingParamsState,
  );
  const { saveApiKey, loading } = useSaveApiKey();
  const {
    visible: apiKeyVisible,
    hideModal: hideApiKeyModal,
    showModal: showApiKeyModal,
  } = useSetModalState();

  const onApiKeySavingOk = useCallback(
    async (postBody: ApiKeyPostBody) => {
      const ret = await saveApiKey({
        ...savingParams,
        ...postBody,
      });

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

  return {
    saveApiKeyLoading: loading,
    initialApiKey: '',
    llmFactory: savingParams.llm_factory,
    onApiKeySavingOk,
    apiKeyVisible,
    hideApiKeyModal,
    showApiKeyModal: onShowApiKeyModal,
  };
};

export const useSubmitSystemModelSetting = () => {
  const { data: systemSetting } = useFetchTenantInfo();
  const { saveTenantInfo: saveSystemModelSetting, loading } =
    useSaveTenantInfo();
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
  const { data: systemSetting } = useFetchTenantInfo();
  const allOptions = useSelectLlmOptionsByModelType();

  return { systemSetting, allOptions };
};

export const useSubmitOllama = () => {
  const [selectedLlmFactory, setSelectedLlmFactory] = useState<string>('');
  const { addLlm, loading } = useAddLlm();
  const {
    visible: llmAddingVisible,
    hideModal: hideLlmAddingModal,
    showModal: showLlmAddingModal,
  } = useSetModalState();

  const onLlmAddingOk = useCallback(
    async (payload: IAddLlmRequestBody) => {
      const ret = await addLlm(payload);
      if (ret === 0) {
        hideLlmAddingModal();
      }
    },
    [hideLlmAddingModal, addLlm],
  );

  const handleShowLlmAddingModal = (llmFactory: string) => {
    setSelectedLlmFactory(llmFactory);
    showLlmAddingModal();
  };

  return {
    llmAddingLoading: loading,
    onLlmAddingOk,
    llmAddingVisible,
    hideLlmAddingModal,
    showLlmAddingModal: handleShowLlmAddingModal,
    selectedLlmFactory,
  };
};

export const useSubmitVolcEngine = () => {
  const { addLlm, loading } = useAddLlm();
  const {
    visible: volcAddingVisible,
    hideModal: hideVolcAddingModal,
    showModal: showVolcAddingModal,
  } = useSetModalState();

  const onVolcAddingOk = useCallback(
    async (payload: IAddLlmRequestBody) => {
      const ret = await addLlm(payload);
      if (ret === 0) {
        hideVolcAddingModal();
      }
    },
    [hideVolcAddingModal, addLlm],
  );

  return {
    volcAddingLoading: loading,
    onVolcAddingOk,
    volcAddingVisible,
    hideVolcAddingModal,
    showVolcAddingModal,
  };
};

export const useSubmitHunyuan = () => {
  const { addLlm, loading } = useAddLlm();
  const {
    visible: HunyuanAddingVisible,
    hideModal: hideHunyuanAddingModal,
    showModal: showHunyuanAddingModal,
  } = useSetModalState();

  const onHunyuanAddingOk = useCallback(
    async (payload: IAddLlmRequestBody) => {
      const ret = await addLlm(payload);
      if (ret === 0) {
        hideHunyuanAddingModal();
      }
    },
    [hideHunyuanAddingModal, addLlm],
  );

  return {
    HunyuanAddingLoading: loading,
    onHunyuanAddingOk,
    HunyuanAddingVisible,
    hideHunyuanAddingModal,
    showHunyuanAddingModal,
  };
};

export const useSubmitTencentCloud = () => {
  const { addLlm, loading } = useAddLlm();
  const {
    visible: TencentCloudAddingVisible,
    hideModal: hideTencentCloudAddingModal,
    showModal: showTencentCloudAddingModal,
  } = useSetModalState();

  const onTencentCloudAddingOk = useCallback(
    async (payload: IAddLlmRequestBody) => {
      const ret = await addLlm(payload);
      if (ret === 0) {
        hideTencentCloudAddingModal();
      }
    },
    [hideTencentCloudAddingModal, addLlm],
  );

  return {
    TencentCloudAddingLoading: loading,
    onTencentCloudAddingOk,
    TencentCloudAddingVisible,
    hideTencentCloudAddingModal,
    showTencentCloudAddingModal,
  };
};

export const useSubmitSpark = () => {
  const { addLlm, loading } = useAddLlm();
  const {
    visible: SparkAddingVisible,
    hideModal: hideSparkAddingModal,
    showModal: showSparkAddingModal,
  } = useSetModalState();

  const onSparkAddingOk = useCallback(
    async (payload: IAddLlmRequestBody) => {
      const ret = await addLlm(payload);
      if (ret === 0) {
        hideSparkAddingModal();
      }
    },
    [hideSparkAddingModal, addLlm],
  );

  return {
    SparkAddingLoading: loading,
    onSparkAddingOk,
    SparkAddingVisible,
    hideSparkAddingModal,
    showSparkAddingModal,
  };
};

export const useSubmityiyan = () => {
  const { addLlm, loading } = useAddLlm();
  const {
    visible: yiyanAddingVisible,
    hideModal: hideyiyanAddingModal,
    showModal: showyiyanAddingModal,
  } = useSetModalState();

  const onyiyanAddingOk = useCallback(
    async (payload: IAddLlmRequestBody) => {
      const ret = await addLlm(payload);
      if (ret === 0) {
        hideyiyanAddingModal();
      }
    },
    [hideyiyanAddingModal, addLlm],
  );

  return {
    yiyanAddingLoading: loading,
    onyiyanAddingOk,
    yiyanAddingVisible,
    hideyiyanAddingModal,
    showyiyanAddingModal,
  };
};

export const useSubmitFishAudio = () => {
  const { addLlm, loading } = useAddLlm();
  const {
    visible: FishAudioAddingVisible,
    hideModal: hideFishAudioAddingModal,
    showModal: showFishAudioAddingModal,
  } = useSetModalState();

  const onFishAudioAddingOk = useCallback(
    async (payload: IAddLlmRequestBody) => {
      const ret = await addLlm(payload);
      if (ret === 0) {
        hideFishAudioAddingModal();
      }
    },
    [hideFishAudioAddingModal, addLlm],
  );

  return {
    FishAudioAddingLoading: loading,
    onFishAudioAddingOk,
    FishAudioAddingVisible,
    hideFishAudioAddingModal,
    showFishAudioAddingModal,
  };
};

export const useSubmitGoogle = () => {
  const { addLlm, loading } = useAddLlm();
  const {
    visible: GoogleAddingVisible,
    hideModal: hideGoogleAddingModal,
    showModal: showGoogleAddingModal,
  } = useSetModalState();

  const onGoogleAddingOk = useCallback(
    async (payload: IAddLlmRequestBody) => {
      const ret = await addLlm(payload);
      if (ret === 0) {
        hideGoogleAddingModal();
      }
    },
    [hideGoogleAddingModal, addLlm],
  );

  return {
    GoogleAddingLoading: loading,
    onGoogleAddingOk,
    GoogleAddingVisible,
    hideGoogleAddingModal,
    showGoogleAddingModal,
  };
};

export const useSubmitBedrock = () => {
  const { addLlm, loading } = useAddLlm();
  const {
    visible: bedrockAddingVisible,
    hideModal: hideBedrockAddingModal,
    showModal: showBedrockAddingModal,
  } = useSetModalState();

  const onBedrockAddingOk = useCallback(
    async (payload: IAddLlmRequestBody) => {
      const ret = await addLlm(payload);
      if (ret === 0) {
        hideBedrockAddingModal();
      }
    },
    [hideBedrockAddingModal, addLlm],
  );

  return {
    bedrockAddingLoading: loading,
    onBedrockAddingOk,
    bedrockAddingVisible,
    hideBedrockAddingModal,
    showBedrockAddingModal,
  };
};

export const useSubmitAzure = () => {
  const { addLlm, loading } = useAddLlm();
  const {
    visible: AzureAddingVisible,
    hideModal: hideAzureAddingModal,
    showModal: showAzureAddingModal,
  } = useSetModalState();

  const onAzureAddingOk = useCallback(
    async (payload: IAddLlmRequestBody) => {
      const ret = await addLlm(payload);
      if (ret === 0) {
        hideAzureAddingModal();
      }
    },
    [hideAzureAddingModal, addLlm],
  );

  return {
    AzureAddingLoading: loading,
    onAzureAddingOk,
    AzureAddingVisible,
    hideAzureAddingModal,
    showAzureAddingModal,
  };
};

export const useHandleDeleteLlm = (llmFactory: string) => {
  const { deleteLlm } = useDeleteLlm();
  const showDeleteConfirm = useShowDeleteConfirm();

  const handleDeleteLlm = (name: string) => () => {
    showDeleteConfirm({
      onOk: async () => {
        deleteLlm({ llm_factory: llmFactory, llm_name: name });
      },
    });
  };

  return { handleDeleteLlm };
};

export const useHandleDeleteFactory = (llmFactory: string) => {
  const { deleteFactory } = useDeleteFactory();
  const showDeleteConfirm = useShowDeleteConfirm();

  const handleDeleteFactory = () => {
    showDeleteConfirm({
      onOk: async () => {
        deleteFactory({ llm_factory: llmFactory });
      },
    });
  };

  return { handleDeleteFactory };
};
