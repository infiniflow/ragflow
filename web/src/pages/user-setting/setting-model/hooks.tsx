import { LLMFactory } from '@/constants/llm';
import { useSetModalState, useShowDeleteConfirm } from '@/hooks/common-hooks';
import {
  IApiKeySavingParams,
  ISystemModelSettingSavingParams,
  useAddLlm,
  useDeleteFactory,
  useDeleteLlm,
  useEnableLlm,
  useSaveApiKey,
  useSaveTenantInfo,
  useSelectLlmOptionsByModelType,
} from '@/hooks/use-llm-request';
import { useFetchTenantInfo } from '@/hooks/use-user-setting-request';
import { IAddLlmRequestBody } from '@/interfaces/request/llm';
import { getRealModelName } from '@/utils/llm-util';
import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useState } from 'react';
import { ApiKeyPostBody } from '../interface';
import { MinerUFormValues } from './modal/mineru-modal';

type SavingParamsState = Omit<IApiKeySavingParams, 'api_key'>;
export type VerifyResult = {
  isValid: boolean | null;
  logs: string;
};
export const useSubmitApiKey = () => {
  const [savingParams, setSavingParams] = useState<SavingParamsState>(
    {} as SavingParamsState,
  );
  const [editMode, setEditMode] = useState(false);
  const { saveApiKey } = useSaveApiKey();
  const [saveLoading, setSaveLoading] = useState(false);
  const {
    visible: apiKeyVisible,
    hideModal: hideApiKeyModal,
    showModal: showApiKeyModal,
  } = useSetModalState();
  const queryClient = useQueryClient();

  const onApiKeySavingOk = useCallback(
    async (postBody: ApiKeyPostBody, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const ret = await saveApiKey({
        ...savingParams,
        ...postBody,
        verify: isVerify,
      });
      if (!isVerify) {
        setSaveLoading(false);
        if (ret.code === 0) {
          queryClient.invalidateQueries({ queryKey: ['llmList'] });
          hideApiKeyModal();
          setEditMode(false);
        }
      }
      if (isVerify) {
        let res = {} as VerifyResult;
        if (ret.data?.success) {
          res = {
            isValid: true,
            logs: ret.data?.message,
          };
        } else {
          res = {
            isValid: false,
            logs: ret.data.message,
          };
        }
        return res;
      }
    },
    [hideApiKeyModal, saveApiKey, savingParams, queryClient],
  );

  const onShowApiKeyModal = useCallback(
    (savingParams: SavingParamsState, isEdit = false) => {
      setSavingParams(savingParams);
      setEditMode(isEdit);
      showApiKeyModal();
    },
    [showApiKeyModal, setSavingParams],
  );

  return {
    saveApiKeyLoading: saveLoading,
    initialApiKey: '',
    llmFactory: savingParams.llm_factory,
    editMode,
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
  const [editMode, setEditMode] = useState(false);
  const [initialValues, setInitialValues] = useState<
    Partial<IAddLlmRequestBody> & { provider_order?: string }
  >();
  const [saveLoading, setSaveLoading] = useState(false);
  const { addLlm } = useAddLlm();
  const {
    visible: llmAddingVisible,
    hideModal: hideLlmAddingModal,
    showModal: showLlmAddingModal,
  } = useSetModalState();

  const onLlmAddingOk = useCallback(
    async (payload: IAddLlmRequestBody, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const cleanedPayload = { ...payload };
      if (!cleanedPayload.api_key || cleanedPayload.api_key.trim() === '') {
        delete cleanedPayload.api_key;
      }

      const ret = await addLlm({ ...cleanedPayload, verify: isVerify });
      if (!isVerify) {
        setSaveLoading(false);
        if (ret.code === 0) {
          hideLlmAddingModal();
          setEditMode(false);
          setInitialValues(undefined);
        }
      }
      if (isVerify) {
        let res = {} as VerifyResult;
        if (ret.data?.success) {
          res = {
            isValid: true,
            logs: ret.data?.message,
          };
        } else {
          res = {
            isValid: false,
            logs: ret.data?.message,
          };
        }
        return res;
      }
    },
    [hideLlmAddingModal, addLlm, setSaveLoading],
  );

  const handleShowLlmAddingModal = (
    llmFactory: string,
    isEdit = false,
    modelData?: any,
    detailedData?: any,
  ) => {
    setSelectedLlmFactory(llmFactory);
    setEditMode(isEdit);

    if (isEdit && detailedData) {
      const initialVals = {
        llm_name: getRealModelName(detailedData.name),
        model_type: detailedData.type,
        api_base: detailedData.api_base || '',
        max_tokens: detailedData.max_tokens || 8192,
        api_key: '',
      };
      setInitialValues(initialVals);
    } else {
      setInitialValues(undefined);
    }
    showLlmAddingModal();
  };

  return {
    llmAddingLoading: saveLoading,
    editMode,
    initialValues,
    onLlmAddingOk,
    llmAddingVisible,
    hideLlmAddingModal,
    showLlmAddingModal: handleShowLlmAddingModal,
    selectedLlmFactory,
  };
};

export const useSubmitVolcEngine = () => {
  const [saveLoading, setSaveLoading] = useState(false);
  const { addLlm } = useAddLlm();
  const {
    visible: volcAddingVisible,
    hideModal: hideVolcAddingModal,
    showModal: showVolcAddingModal,
  } = useSetModalState();

  const onVolcAddingOk = useCallback(
    async (payload: IAddLlmRequestBody, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const ret = await addLlm({ ...payload, verify: isVerify });
      if (!isVerify) {
        setSaveLoading(false);
        if (ret.code === 0) {
          hideVolcAddingModal();
        }
      }
      if (isVerify) {
        let res = {} as VerifyResult;
        if (ret.data?.success) {
          res = {
            isValid: true,
            logs: ret.data?.message,
          };
        } else {
          res = {
            isValid: false,
            logs: ret.data?.message,
          };
        }
        return res;
      }
    },
    [hideVolcAddingModal, addLlm, setSaveLoading],
  );

  return {
    volcAddingLoading: saveLoading,
    onVolcAddingOk,
    volcAddingVisible,
    hideVolcAddingModal,
    showVolcAddingModal,
  };
};

export const useSubmitTencentCloud = () => {
  const [saveLoading, setSaveLoading] = useState(false);
  const { addLlm } = useAddLlm();
  const {
    visible: TencentCloudAddingVisible,
    hideModal: hideTencentCloudAddingModal,
    showModal: showTencentCloudAddingModal,
  } = useSetModalState();

  const onTencentCloudAddingOk = useCallback(
    async (payload: IAddLlmRequestBody, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const ret = await addLlm({ ...payload, verify: isVerify });
      if (!isVerify) {
        setSaveLoading(false);
        if (ret.code === 0) {
          hideTencentCloudAddingModal();
        }
      }
      if (isVerify) {
        let res = {} as VerifyResult;
        if (ret.data?.success) {
          res = {
            isValid: true,
            logs: ret.data?.message,
          };
        } else {
          res = {
            isValid: false,
            logs: ret.data?.message,
          };
        }
        return res;
      }
    },
    [hideTencentCloudAddingModal, addLlm, setSaveLoading],
  );

  return {
    TencentCloudAddingLoading: saveLoading,
    onTencentCloudAddingOk,
    TencentCloudAddingVisible,
    hideTencentCloudAddingModal,
    showTencentCloudAddingModal,
  };
};

export const useSubmitSpark = () => {
  const [saveLoading, setSaveLoading] = useState(false);
  const { addLlm } = useAddLlm();
  const {
    visible: SparkAddingVisible,
    hideModal: hideSparkAddingModal,
    showModal: showSparkAddingModal,
  } = useSetModalState();

  const onSparkAddingOk = useCallback(
    async (payload: IAddLlmRequestBody, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const ret = await addLlm({ ...payload, verify: isVerify });
      if (!isVerify) {
        setSaveLoading(false);
        if (ret.code === 0) {
          hideSparkAddingModal();
        }
      }
      if (isVerify) {
        let res = {} as VerifyResult;
        if (ret.data?.success) {
          res = {
            isValid: true,
            logs: ret.data?.message,
          };
        } else {
          res = {
            isValid: false,
            logs: ret.data?.message,
          };
        }
        return res;
      }
    },
    [hideSparkAddingModal, addLlm, setSaveLoading],
  );

  return {
    SparkAddingLoading: saveLoading,
    onSparkAddingOk,
    SparkAddingVisible,
    hideSparkAddingModal,
    showSparkAddingModal,
  };
};

export const useSubmityiyan = () => {
  const [saveLoading, setSaveLoading] = useState(false);
  const { addLlm } = useAddLlm();
  const {
    visible: yiyanAddingVisible,
    hideModal: hideyiyanAddingModal,
    showModal: showyiyanAddingModal,
  } = useSetModalState();

  const onyiyanAddingOk = useCallback(
    async (payload: IAddLlmRequestBody, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const ret = await addLlm({ ...payload, verify: isVerify });
      if (!isVerify) {
        setSaveLoading(false);
        if (ret.code === 0) {
          hideyiyanAddingModal();
        }
      }
      if (isVerify) {
        let res = {} as VerifyResult;
        if (ret.data?.success) {
          res = {
            isValid: true,
            logs: ret.data?.message,
          };
        } else {
          res = {
            isValid: false,
            logs: ret.data?.message,
          };
        }
        return res;
      }
    },
    [hideyiyanAddingModal, addLlm, setSaveLoading],
  );

  return {
    yiyanAddingLoading: saveLoading,
    onyiyanAddingOk,
    yiyanAddingVisible,
    hideyiyanAddingModal,
    showyiyanAddingModal,
  };
};

export const useSubmitFishAudio = () => {
  const [saveLoading, setSaveLoading] = useState(false);
  const { addLlm } = useAddLlm();
  const {
    visible: FishAudioAddingVisible,
    hideModal: hideFishAudioAddingModal,
    showModal: showFishAudioAddingModal,
  } = useSetModalState();

  const onFishAudioAddingOk = useCallback(
    async (payload: IAddLlmRequestBody, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const ret = await addLlm({ ...payload, verify: isVerify });
      if (!isVerify) {
        setSaveLoading(false);
        if (ret.code === 0) {
          hideFishAudioAddingModal();
        }
      }
      if (isVerify) {
        let res = {} as VerifyResult;
        if (ret.data?.success) {
          res = {
            isValid: true,
            logs: ret.data?.message,
          };
        } else {
          res = {
            isValid: false,
            logs: ret.data?.message,
          };
        }
        return res;
      }
    },
    [hideFishAudioAddingModal, addLlm, setSaveLoading],
  );

  return {
    FishAudioAddingLoading: saveLoading,
    onFishAudioAddingOk,
    FishAudioAddingVisible,
    hideFishAudioAddingModal,
    showFishAudioAddingModal,
  };
};

export const useSubmitGoogle = () => {
  const [saveLoading, setSaveLoading] = useState(false);
  const { addLlm } = useAddLlm();
  const {
    visible: GoogleAddingVisible,
    hideModal: hideGoogleAddingModal,
    showModal: showGoogleAddingModal,
  } = useSetModalState();

  const onGoogleAddingOk = useCallback(
    async (payload: IAddLlmRequestBody, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const ret = await addLlm({ ...payload, verify: isVerify });
      if (!isVerify) {
        setSaveLoading(false);
        if (ret.code === 0) {
          hideGoogleAddingModal();
        }
      }
      if (isVerify) {
        let res = {} as VerifyResult;
        if (ret.data?.success) {
          res = {
            isValid: true,
            logs: ret.data?.message,
          };
        } else {
          res = {
            isValid: false,
            logs: ret.data?.message,
          };
        }
        return res;
      }
    },
    [hideGoogleAddingModal, addLlm, setSaveLoading],
  );

  return {
    GoogleAddingLoading: saveLoading,
    onGoogleAddingOk,
    GoogleAddingVisible,
    hideGoogleAddingModal,
    showGoogleAddingModal,
  };
};

export const useSubmitBedrock = () => {
  const [saveLoading, setSaveLoading] = useState(false);
  const { addLlm } = useAddLlm();
  const {
    visible: bedrockAddingVisible,
    hideModal: hideBedrockAddingModal,
    showModal: showBedrockAddingModal,
  } = useSetModalState();

  const onBedrockAddingOk = useCallback(
    async (payload: IAddLlmRequestBody, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const ret = await addLlm({ ...payload, verify: isVerify });
      if (!isVerify) {
        setSaveLoading(false);
        if (ret.code === 0) {
          hideBedrockAddingModal();
        }
      }
      if (isVerify) {
        let res = {} as VerifyResult;
        if (ret.data?.success) {
          res = {
            isValid: true,
            logs: ret.data?.message,
          };
        } else {
          res = {
            isValid: false,
            logs: ret.data?.message,
          };
        }
        return res;
      }
    },
    [hideBedrockAddingModal, addLlm, setSaveLoading],
  );

  return {
    bedrockAddingLoading: saveLoading,
    onBedrockAddingOk,
    bedrockAddingVisible,
    hideBedrockAddingModal,
    showBedrockAddingModal,
  };
};

export const useSubmitAzure = () => {
  const [saveLoading, setSaveLoading] = useState(false);
  const { addLlm } = useAddLlm();
  const {
    visible: AzureAddingVisible,
    hideModal: hideAzureAddingModal,
    showModal: showAzureAddingModal,
  } = useSetModalState();

  const onAzureAddingOk = useCallback(
    async (payload: IAddLlmRequestBody, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const ret = await addLlm({ ...payload, verify: isVerify });
      if (!isVerify) {
        setSaveLoading(false);
        if (ret.code === 0) {
          hideAzureAddingModal();
        }
      }
      if (isVerify) {
        let res = {} as VerifyResult;
        if (ret.data?.success) {
          res = {
            isValid: true,
            logs: ret.data?.message,
          };
        } else {
          res = {
            isValid: false,
            logs: ret.data?.message,
          };
        }
        return res;
      }
    },
    [hideAzureAddingModal, addLlm, setSaveLoading],
  );

  return {
    AzureAddingLoading: saveLoading,
    onAzureAddingOk,
    AzureAddingVisible,
    hideAzureAddingModal,
    showAzureAddingModal,
  };
};

export const useHandleDeleteLlm = (llmFactory: string) => {
  const { deleteLlm } = useDeleteLlm();
  const showDeleteConfirm = useShowDeleteConfirm();

  const handleDeleteLlm = (name: string) => {
    showDeleteConfirm({
      onOk: async () => {
        deleteLlm({ llm_factory: llmFactory, llm_name: name });
      },
    });
  };

  return { handleDeleteLlm };
};

export const useHandleEnableLlm = (llmFactory: string) => {
  const { enableLlm } = useEnableLlm();

  const handleEnableLlm = (name: string, enable: boolean) => {
    enableLlm({ llm_factory: llmFactory, llm_name: name, enable });
  };

  return { handleEnableLlm };
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

  return { handleDeleteFactory, deleteFactory };
};

export const useSubmitMinerU = () => {
  const [saveLoading, setSaveLoading] = useState(false);
  const { addLlm } = useAddLlm();
  const {
    visible: mineruVisible,
    hideModal: hideMineruModal,
    showModal: showMineruModal,
  } = useSetModalState();

  const onMineruOk = useCallback(
    async (payload: MinerUFormValues, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const cfg: any = {
        ...payload,
        mineru_delete_output:
          (payload.mineru_delete_output ?? true) ? '1' : '0',
      };
      if (payload.mineru_backend !== 'vlm-http-client') {
        delete cfg.mineru_server_url;
      }
      const req: IAddLlmRequestBody = {
        llm_factory: LLMFactory.MinerU,
        llm_name: payload.llm_name,
        model_type: 'ocr',
        api_key: cfg,
        api_base: '',
        max_tokens: 0,
      };
      const ret = await addLlm({ ...req, verify: isVerify });
      if (!isVerify) {
        setSaveLoading(false);
        if (ret.code === 0) {
          hideMineruModal();
        }
      }
      if (isVerify) {
        let res = {} as VerifyResult;
        if (ret.data?.success) {
          res = {
            isValid: true,
            logs: ret.data?.message,
          };
        } else {
          res = {
            isValid: false,
            logs: ret.data?.message,
          };
        }
        return res;
      }
    },
    [addLlm, hideMineruModal, setSaveLoading],
  );

  return {
    mineruVisible,
    hideMineruModal,
    showMineruModal,
    onMineruOk,
    mineruLoading: saveLoading,
  };
};

export const useSubmitPaddleOCR = () => {
  const [saveLoading, setSaveLoading] = useState(false);
  const { addLlm } = useAddLlm();
  const {
    visible: paddleocrVisible,
    hideModal: hidePaddleOCRModal,
    showModal: showPaddleOCRModal,
  } = useSetModalState();

  const onPaddleOCROk = useCallback(
    async (payload: any, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const cfg: any = {
        ...payload,
      };
      const req: IAddLlmRequestBody = {
        llm_factory: LLMFactory.PaddleOCR,
        llm_name: payload.llm_name,
        model_type: 'ocr',
        api_key: cfg,
        api_base: '',
        max_tokens: 0,
      };
      const ret = await addLlm({ ...req, verify: isVerify });
      if (!isVerify) {
        setSaveLoading(false);
        if (ret.code === 0) {
          hidePaddleOCRModal();
          return true;
        }
      }
      if (isVerify) {
        let res = {} as VerifyResult;
        if (ret.data?.success) {
          res = {
            isValid: true,
            logs: ret.data?.message,
          };
        } else {
          res = {
            isValid: false,
            logs: ret.data?.message,
          };
        }
        return res;
      }
      return false;
    },
    [addLlm, hidePaddleOCRModal, setSaveLoading],
  );

  return {
    paddleocrVisible,
    hidePaddleOCRModal,
    showPaddleOCRModal,
    onPaddleOCROk,
    paddleocrLoading: saveLoading,
  };
};

export const useVerifySettings = ({
  onVerify,
}: {
  onVerify:
    | ((
        postBody: ApiKeyPostBody,
        isVerify?: boolean,
      ) => Promise<VerifyResult | undefined>)
    | ((
        payload: IAddLlmRequestBody,
        isVerify?: boolean,
      ) => Promise<VerifyResult | undefined>)
    | ((
        payload: MinerUFormValues,
        isVerify?: boolean,
      ) => Promise<VerifyResult | undefined>)
    | ((payload: any, isVerify?: boolean) => Promise<boolean | VerifyResult>)
    | (() => void);
}) => {
  const onApiKeyVerifying = useCallback(
    async (postBody: any) => {
      const res = await onVerify(postBody, true);
      return res;
    },
    [onVerify],
  );
  return {
    onApiKeyVerifying,
  };
};
