import { LLMFactory } from '@/constants/llm';
import { useSetModalState } from '@/hooks/common-hooks';
import {
  useAddInstanceModel,
  useAddProviderInstance,
  useFetchAddedProviders,
  useFetchProviderInstances,
} from '@/hooks/use-llm-request';
import { IAddProviderInstanceRequestBody } from '@/interfaces/request/llm';
import { getRealModelName } from '@/utils/llm-util';
import { useCallback, useMemo, useState } from 'react';
import { ApiKeyPostBody } from '../interface';
import { MinerUFormValues } from './modal/mineru-modal';
import { splitProviderPayload } from './payload-utils';

type SavingParamsState = {
  llm_factory: string;
  llm_name?: string;
  model_type?: string;
  instance_name?: string;
  base_url?: string;
};
export type VerifyResult = {
  isValid: boolean | null;
  logs: string;
};

const useSubmitProviderInstance = () => {
  const { addProviderInstance } = useAddProviderInstance();
  const { addInstanceModel } = useAddInstanceModel();

  return useCallback(
    async (payload: IAddProviderInstanceRequestBody, isVerify = false) => {
      if (isVerify) {
        return addProviderInstance({ ...payload, verify: true });
      }

      const { instancePayload, modelPayload } = splitProviderPayload(payload);
      const hasModelPayload =
        !!modelPayload.model_name && !!modelPayload.model_type;

      const instanceRet = await addProviderInstance({
        ...instancePayload,
        llm_factory: payload.llm_factory,
        instance_name: payload.instance_name,
      } as IAddProviderInstanceRequestBody);
      if (instanceRet.code !== 0 || !hasModelPayload) {
        return instanceRet;
      }

      if (!hasModelPayload) {
        return { code: 0, data: null } as any;
      }

      return addInstanceModel({
        provider_name: payload.llm_factory,
        instance_name: payload.instance_name,
        ...modelPayload,
      });
    },
    [addProviderInstance, addInstanceModel],
  );
};

export const useFetchInstanceNameSet = (providerName: string) => {
  const { data: addedProviders } = useFetchAddedProviders();
  const providerExists = useMemo(
    () => addedProviders.some((p) => p.name === providerName),
    [addedProviders, providerName],
  );
  const { data: instances } = useFetchProviderInstances(
    providerExists ? providerName : '',
  );
  const instanceNameSet = useMemo(
    () => new Set(instances.map((i) => i.instance_name)),
    [instances],
  );
  return { instanceNameSet, providerExists };
};

export const useHideWhenInstanceExists = (instanceNameSet: Set<string>) => {
  return useCallback(
    (formValues: any) => {
      const name = ((formValues?.instance_name as string) || '').trim();
      return !(name && instanceNameSet.has(name));
    },
    [instanceNameSet],
  );
};
export const useSubmitApiKey = () => {
  const [savingParams, setSavingParams] = useState<SavingParamsState>(
    {} as SavingParamsState,
  );
  const [editMode, setEditMode] = useState(false);
  const submitProviderInstance = useSubmitProviderInstance();
  const [saveLoading, setSaveLoading] = useState(false);
  const {
    visible: apiKeyVisible,
    hideModal: hideApiKeyModal,
    showModal: showApiKeyModal,
  } = useSetModalState();

  const onApiKeySavingOk = useCallback(
    async (postBody: ApiKeyPostBody, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      let apiKey: string | Record<string, any> = postBody.api_key || '';

      if (savingParams.llm_factory === LLMFactory.SILICONFLOW) {
        let sourceFid: string = LLMFactory.SILICONFLOW;
        const baseUrl = postBody.base_url;
        if (baseUrl) {
          try {
            const parsed = new URL(baseUrl);
            const host = parsed.hostname.toLowerCase();
            if (
              host === 'api.siliconflow.com' ||
              host.endsWith('.api.siliconflow.com')
            ) {
              sourceFid = 'siliconflow_intl';
            }
          } catch {
            // ignore invalid URL and keep default sourceFid
          }
        }
        apiKey = { api_key: postBody.api_key, source_fid: sourceFid };
      }

      const req: IAddProviderInstanceRequestBody = {
        instance_name:
          postBody.instance_name || savingParams.instance_name || '',
        llm_factory: savingParams.llm_factory,
        llm_name: savingParams.llm_name || '',
        model_type: savingParams.model_type || '',
        api_key: apiKey,
        api_base: postBody.base_url || '',
        max_tokens: 0,
      };

      const ret = await submitProviderInstance(req, isVerify);
      if (!isVerify) {
        setSaveLoading(false);
        if (ret.code === 0) {
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
    [hideApiKeyModal, submitProviderInstance, savingParams],
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

export const useSubmitOllama = () => {
  const [selectedLlmFactory, setSelectedLlmFactory] = useState<string>('');
  const [editMode, setEditMode] = useState(false);
  const [initialValues, setInitialValues] = useState<
    Partial<IAddProviderInstanceRequestBody> & { provider_order?: string }
  >();
  const [saveLoading, setSaveLoading] = useState(false);
  const submitProviderInstance = useSubmitProviderInstance();
  const {
    visible: llmAddingVisible,
    hideModal: hideLlmAddingModal,
    showModal: showLlmAddingModal,
  } = useSetModalState();

  const onLlmAddingOk = useCallback(
    async (payload: IAddProviderInstanceRequestBody, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const cleanedPayload = { ...payload };
      // if (
      //   !cleanedPayload.api_key ||
      //   (typeof cleanedPayload.api_key === 'string' &&
      //     cleanedPayload.api_key.trim() === '')
      // ) {
      //   delete cleanedPayload.api_key;
      // }

      const ret = await submitProviderInstance(cleanedPayload, isVerify);
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
    [hideLlmAddingModal, submitProviderInstance, setSaveLoading],
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
        instance_name:
          detailedData.instance_name || getRealModelName(detailedData.name),
        llm_name: getRealModelName(detailedData.name),
        model_type: detailedData.type,
        api_base: detailedData.api_base || '',
        max_tokens: detailedData.max_tokens || 8192,
        api_key: '',
        is_tools: detailedData.is_tools || false,
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
  const submitProviderInstance = useSubmitProviderInstance();
  const {
    visible: volcAddingVisible,
    hideModal: hideVolcAddingModal,
    showModal: showVolcAddingModal,
  } = useSetModalState();

  const onVolcAddingOk = useCallback(
    async (payload: IAddProviderInstanceRequestBody, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const ret = await submitProviderInstance(payload, isVerify);
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
    [hideVolcAddingModal, submitProviderInstance, setSaveLoading],
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
  const submitProviderInstance = useSubmitProviderInstance();
  const {
    visible: TencentCloudAddingVisible,
    hideModal: hideTencentCloudAddingModal,
    showModal: showTencentCloudAddingModal,
  } = useSetModalState();

  const onTencentCloudAddingOk = useCallback(
    async (payload: IAddProviderInstanceRequestBody, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const ret = await submitProviderInstance(payload, isVerify);
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
    [hideTencentCloudAddingModal, submitProviderInstance, setSaveLoading],
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
  const submitProviderInstance = useSubmitProviderInstance();
  const {
    visible: SparkAddingVisible,
    hideModal: hideSparkAddingModal,
    showModal: showSparkAddingModal,
  } = useSetModalState();

  const onSparkAddingOk = useCallback(
    async (payload: IAddProviderInstanceRequestBody, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const ret = await submitProviderInstance(payload, isVerify);
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
    [hideSparkAddingModal, submitProviderInstance, setSaveLoading],
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
  const submitProviderInstance = useSubmitProviderInstance();
  const {
    visible: yiyanAddingVisible,
    hideModal: hideyiyanAddingModal,
    showModal: showyiyanAddingModal,
  } = useSetModalState();

  const onyiyanAddingOk = useCallback(
    async (payload: IAddProviderInstanceRequestBody, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const ret = await submitProviderInstance(payload, isVerify);
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
    [hideyiyanAddingModal, submitProviderInstance, setSaveLoading],
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
  const submitProviderInstance = useSubmitProviderInstance();
  const {
    visible: FishAudioAddingVisible,
    hideModal: hideFishAudioAddingModal,
    showModal: showFishAudioAddingModal,
  } = useSetModalState();

  const onFishAudioAddingOk = useCallback(
    async (payload: IAddProviderInstanceRequestBody, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const ret = await submitProviderInstance(payload, isVerify);
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
    [hideFishAudioAddingModal, submitProviderInstance, setSaveLoading],
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
  const submitProviderInstance = useSubmitProviderInstance();
  const {
    visible: GoogleAddingVisible,
    hideModal: hideGoogleAddingModal,
    showModal: showGoogleAddingModal,
  } = useSetModalState();

  const onGoogleAddingOk = useCallback(
    async (payload: IAddProviderInstanceRequestBody, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const ret = await submitProviderInstance(payload, isVerify);
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
    [hideGoogleAddingModal, submitProviderInstance, setSaveLoading],
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
  const submitProviderInstance = useSubmitProviderInstance();
  const {
    visible: bedrockAddingVisible,
    hideModal: hideBedrockAddingModal,
    showModal: showBedrockAddingModal,
  } = useSetModalState();

  const onBedrockAddingOk = useCallback(
    async (payload: IAddProviderInstanceRequestBody, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const ret = await submitProviderInstance(payload, isVerify);
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
    [hideBedrockAddingModal, submitProviderInstance, setSaveLoading],
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
  const submitProviderInstance = useSubmitProviderInstance();
  const {
    visible: AzureAddingVisible,
    hideModal: hideAzureAddingModal,
    showModal: showAzureAddingModal,
  } = useSetModalState();

  const onAzureAddingOk = useCallback(
    async (payload: IAddProviderInstanceRequestBody, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const ret = await submitProviderInstance(payload, isVerify);
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
    [hideAzureAddingModal, submitProviderInstance, setSaveLoading],
  );

  return {
    AzureAddingLoading: saveLoading,
    onAzureAddingOk,
    AzureAddingVisible,
    hideAzureAddingModal,
    showAzureAddingModal,
  };
};

export const useSubmitMinerU = () => {
  const [saveLoading, setSaveLoading] = useState(false);
  const submitProviderInstance = useSubmitProviderInstance();
  const {
    visible: mineruVisible,
    hideModal: hideMineruModal,
    showModal: showMineruModal,
  } = useSetModalState();

  const onMineruOk = useCallback(
    async (
      payload: MinerUFormValues & { instance_name: string },
      isVerify = false,
    ) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const cfg: any = {
        ...payload,
        mineru_delete_output:
          (payload.mineru_delete_output ?? true) ? '1' : '0',
      };
      delete cfg.instance_name;
      if (payload.mineru_backend !== 'vlm-http-client') {
        delete cfg.mineru_server_url;
      }
      const req: IAddProviderInstanceRequestBody = {
        instance_name: payload.instance_name,
        llm_factory: LLMFactory.MinerU,
        llm_name: payload.llm_name,
        model_type: 'ocr',
        api_key: cfg,
        api_base: '',
        max_tokens: 0,
      };
      const ret = await submitProviderInstance(req, isVerify);
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
    [submitProviderInstance, hideMineruModal, setSaveLoading],
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
  const submitProviderInstance = useSubmitProviderInstance();
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
      delete cfg.instance_name;
      const req: IAddProviderInstanceRequestBody = {
        instance_name: payload.instance_name,
        llm_factory: LLMFactory.PaddleOCR,
        llm_name: payload.llm_name,
        model_type: 'ocr',
        api_key: cfg,
        api_base: '',
        max_tokens: 0,
      };
      const ret = await submitProviderInstance(req, isVerify);
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
    [submitProviderInstance, hidePaddleOCRModal, setSaveLoading],
  );

  return {
    paddleocrVisible,
    hidePaddleOCRModal,
    showPaddleOCRModal,
    onPaddleOCROk,
    paddleocrLoading: saveLoading,
  };
};

export const useSubmitOpenDataLoader = () => {
  const [saveLoading, setSaveLoading] = useState(false);
  const submitProviderInstance = useSubmitProviderInstance();
  const {
    visible: opendataloaderVisible,
    hideModal: hideOpenDataLoaderModal,
    showModal: showOpenDataLoaderModal,
  } = useSetModalState();

  const onOpenDataLoaderOk = useCallback(
    async (payload: any, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const cfg: any = { ...payload };
      delete cfg.instance_name;
      const req: IAddProviderInstanceRequestBody = {
        instance_name: payload.instance_name,
        llm_factory: LLMFactory.OpenDataLoader,
        llm_name: payload.llm_name,
        model_type: 'ocr',
        api_key: cfg,
        api_base: '',
        max_tokens: 0,
      };
      const ret = await submitProviderInstance(req, isVerify);
      if (!isVerify) {
        setSaveLoading(false);
        if (ret.code === 0) {
          hideOpenDataLoaderModal();
          return true;
        }
      }
      if (isVerify) {
        return {
          isValid: !!ret.data?.success,
          logs: ret.data?.message,
        } as VerifyResult;
      }
      return false;
    },
    [submitProviderInstance, hideOpenDataLoaderModal, setSaveLoading],
  );

  return {
    opendataloaderVisible,
    hideOpenDataLoaderModal,
    showOpenDataLoaderModal,
    onOpenDataLoaderOk,
    opendataloaderLoading: saveLoading,
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
        payload: IAddProviderInstanceRequestBody,
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
