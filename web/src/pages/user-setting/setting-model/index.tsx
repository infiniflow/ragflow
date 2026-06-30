import Spotlight from '@/components/spotlight';
import { LLMFactory } from '@/constants/llm';
import {
  useAddInstanceModel,
  useAddProviderInstance,
  useFetchAvailableProviders,
  useVerifyProviderConnection,
} from '@/hooks/use-llm-request';
import { IInstanceModel, IProviderInstance } from '@/interfaces/database/llm';
import type {
  IAddProviderInstanceRequestBody,
  IModelInfo,
} from '@/interfaces/request/llm';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { isLocalLlmFactory } from '../utils';
import SystemSetting from './components/system-setting';
import { AvailableModels } from './components/un-add-model';
import { UsedModel } from './components/used-model';
import { useSubmitBedrock, useSubmitSoMark, useVerifySettings } from './hooks';
import BedrockModal from './modal/bedrock-modal';
import ProviderModal, { IViewModeOkPayload } from './modal/provider-modal';
import SoMarkModal from './modal/somark-modal';
import { splitProviderPayload } from './payload-utils';

const ModelProviders = () => {
  // Retained special modals
  const {
    bedrockAddingLoading,
    onBedrockAddingOk,
    bedrockAddingVisible,
    hideBedrockAddingModal,
    showBedrockAddingModal,
  } = useSubmitBedrock();

  // Unified ProviderModal state
  const [providerVisible, setProviderVisible] = useState(false);
  const [currentLlmFactory, setCurrentLlmFactory] = useState<string>('');
  const [providerLoading, setProviderLoading] = useState(false);

  // viewMode (edit-models) state: when true, ProviderModal opens in
  // read-only mode for everything except the model-related fields.
  // `viewModeInitialValues` carries the existing instance + model data.
  const [viewMode, setViewMode] = useState(false);
  const [viewModeInitialValues, setViewModeInitialValues] = useState<
    Record<string, any> | undefined
  >(undefined);

  // ProviderModal submission logic: calls addProviderInstance + addInstanceModel
  const { addProviderInstance } = useAddProviderInstance();
  const { addInstanceModel } = useAddInstanceModel();
  const { verifyProviderConnection } = useVerifyProviderConnection();
  const { data: availableProviders } = useFetchAvailableProviders();

  // Convert IAvailableProvider.url to baseUrlOptions
  // IAvailableProvider.url looks like { default?: string; cn?: string; intl?: string; ... }
  // Mapped to [{ value: 'https://...', regionKey: 'default', label: <span>https://...<span>default</span></span> }, ...]
  // `regionKey` carries the original key so the modal can map the currently
  // selected URL back to its key for the `region` submit field.
  const buildBaseUrlOptions = useCallback(
    (urlObj?: Record<string, string | undefined>) => {
      if (!urlObj) return undefined;
      const options = Object.keys(urlObj)
        .filter((k) => !!urlObj[k])
        .map((k) => {
          const v = urlObj[k] as string;
          // if (k === 'default') {
          //   return { value: v, label: v };
          // }
          return {
            value: v,
            regionKey: k,
            label: (
              <div className="flex justify-between items-center gap-2">
                <span className="truncate">{v}</span>
                <span className="text-xs text-text-secondary bg-bg-card px-2 py-0.5 rounded-sm shrink-0">
                  {k}
                </span>
              </div>
            ),
          };
        });
      return options.length > 0 ? options : undefined;
    },
    [],
  );

  // baseUrlOptions for the current factory (looked up from availableProviders)
  const currentProvider = useMemo(
    () =>
      currentLlmFactory
        ? availableProviders.find((p) => p.name === currentLlmFactory)
        : undefined,
    [availableProviders, currentLlmFactory],
  );
  const currentBaseUrlOptions = useMemo(
    () => buildBaseUrlOptions(currentProvider?.url),
    [buildBaseUrlOptions, currentProvider],
  );

  const handleProviderOk = useCallback(
    async (payload: IAddProviderInstanceRequestBody, isVerify = false) => {
      if (!isVerify) setProviderLoading(true);
      try {
        if (isVerify) {
          // Verify mode: call verify API
          const ret = await addProviderInstance({ ...payload, verify: true });
          return ret;
        }
        // Normal submission
        const { instancePayload, modelPayload } = splitProviderPayload(payload);
        const hasModelPayload =
          !!modelPayload.model_name && !!modelPayload.model_type;
        const instanceRet = await addProviderInstance({
          ...instancePayload,
          llm_factory: payload.llm_factory,
          instance_name: payload.instance_name,
        } as IAddProviderInstanceRequestBody);
        if (instanceRet.code !== 0) {
          return instanceRet;
        }
        // When model information has been submitted nested in the instance via model_info
        // (e.g., VolcEngine / LocalLLM), addInstanceModel is no longer called separately;
        // close the modal directly.
        if (!hasModelPayload) {
          setProviderVisible(false);
          return instanceRet;
        }
        const modelRet = await addInstanceModel({
          provider_name: payload.llm_factory,
          instance_name: payload.instance_name,
          ...modelPayload,
        });
        if (modelRet.code === 0) {
          setProviderVisible(false);
        }
        return modelRet;
      } finally {
        if (!isVerify) setProviderLoading(false);
      }
    },
    [addProviderInstance, addInstanceModel],
  );

  useEffect(() => {
    if (!providerVisible) {
      setProviderLoading(false);
    }
  }, [providerVisible]);

  const handleProviderVerify = useCallback(
    async (params: any) => {
      // ProviderModal's handleVerify flattens verifyArgs onto params
      // verifyArgs comes from config.verifyTransform, fields are apiKey/baseUrl/region/modelInfo
      const apiKey = params.apiKey ?? params.api_key ?? params._apiKey ?? '';
      const baseUrl = params.baseUrl ?? params.base_url ?? params._baseUrl;
      const region = params.region ?? params._region;
      const modelInfo =
        params.modelInfo ?? params.model_info ?? params._modelInfo;
      const ret = await verifyProviderConnection({
        provider_name: params.llm_factory ?? currentLlmFactory,
        api_key: apiKey,
        base_url: baseUrl,
        region: region,
        model_info: modelInfo,
      });
      if (ret.code === 0) {
        return { isValid: true, logs: ret.message };
      }
      return { isValid: false, logs: ret.message };
    },
    [verifyProviderConnection, currentLlmFactory],
  );

  const {
    somarkVisible,
    hideSoMarkModal,
    showSoMarkModal,
    onSoMarkOk,
    somarkLoading,
  } = useSubmitSoMark();

  const ModalMap = useMemo(
    () => ({
      [LLMFactory.Bedrock]: showBedrockAddingModal,
      [LLMFactory.VolcEngine]: () => {
        setCurrentLlmFactory(LLMFactory.VolcEngine);
        setProviderVisible(true);
      },
      [LLMFactory.XunFeiSpark]: () => {
        setCurrentLlmFactory(LLMFactory.XunFeiSpark);
        setProviderVisible(true);
      },
      [LLMFactory.BaiduYiYan]: () => {
        setCurrentLlmFactory(LLMFactory.BaiduYiYan);
        setProviderVisible(true);
      },
      [LLMFactory.FishAudio]: () => {
        setCurrentLlmFactory(LLMFactory.FishAudio);
        setProviderVisible(true);
      },
      [LLMFactory.TencentCloud]: () => {
        setCurrentLlmFactory(LLMFactory.TencentCloud);
        setProviderVisible(true);
      },
      [LLMFactory.GoogleCloud]: () => {
        setCurrentLlmFactory(LLMFactory.GoogleCloud);
        setProviderVisible(true);
      },
      [LLMFactory.AzureOpenAI]: () => {
        setCurrentLlmFactory(LLMFactory.AzureOpenAI);
        setProviderVisible(true);
      },
      [LLMFactory.MinerU]: () => {
        setCurrentLlmFactory(LLMFactory.MinerU);
        setProviderVisible(true);
      },
      [LLMFactory.PaddleOCR]: () => {
        setCurrentLlmFactory(LLMFactory.PaddleOCR);
        setProviderVisible(true);
      },
      [LLMFactory.OpenDataLoader]: () => {
        setCurrentLlmFactory(LLMFactory.OpenDataLoader);
        setProviderVisible(true);
      },
      [LLMFactory.SoMark]: showSoMarkModal,
    }),
    [showBedrockAddingModal, showSoMarkModal],
  );

  const handleAddModel = useCallback(
    (llmFactory: string) => {
      if (isLocalLlmFactory(llmFactory)) {
        setCurrentLlmFactory(llmFactory);
        setProviderVisible(true);
      } else if (llmFactory in ModalMap) {
        ModalMap[llmFactory as keyof typeof ModalMap]();
      } else {
        setCurrentLlmFactory(llmFactory);
        setProviderVisible(true);
      }
    },
    [ModalMap],
  );

  // Open the ProviderModal in viewMode (read-only) for an existing
  // instance so the user can edit its model list. The instance's
  // `api_key`, `baseUrl` and `model_info` are passed as initial values;
  // the list picker uses `model_info` to pre-check the already-added
  // models.
  const handleEditInstance = useCallback(
    (
      providerName: string,
      instance: IProviderInstance,
      models: IInstanceModel[],
    ) => {
      setCurrentLlmFactory(providerName);
      const modelInfos: IModelInfo[] = models.map((m) => ({
        model_name: m.name,
        model_type: m.model_type,
        max_tokens: m.max_tokens ?? 0,
      }));
      // For non-LIST_MODEL_PROVIDERS, the modal renders model_name /
      // model_type / max_tokens / is_tools as form fields, so seed
      // them from the first existing model to match what the user sees
      // in the instance list.
      const firstModel = models[0];
      setViewModeInitialValues({
        instance_name: instance.instance_name,
        api_key: instance.api_key,
        // baseUrl is only present when the showProviderInstance endpoint
        // returned it; pass it as both `base_url` and `api_base` so it
        // fills the form field regardless of which name the provider
        // config uses.
        ...(instance.base_url
          ? { base_url: instance.base_url, api_base: instance.base_url }
          : {}),
        ...(firstModel
          ? {
              model_name: firstModel.name,
              model_type: firstModel.model_type,
              max_tokens: firstModel.max_tokens,
            }
          : {}),
        model_info: modelInfos,
      });
      setViewMode(true);
      setProviderVisible(true);
    },
    [],
  );

  // viewMode save handler: receives the list of selected models (or
  // the editable model fields for non-LIST_MODEL_PROVIDERS) from the
  // modal and adds them via `addInstanceModel`. Does NOT call
  // `addProviderInstance` because the instance itself is unchanged.
  const handleViewModeOk = useCallback(
    async (payload: IViewModeOkPayload) => {
      setProviderLoading(true);
      try {
        if (payload.modelInfos.length > 0) {
          // LIST_MODEL_PROVIDERS: full sync — call addInstanceModel for
          // every selected model. The backend is idempotent so re-adding
          // an already-present model is a no-op.
          for (const model of payload.modelInfos) {
            const modelType = Array.isArray(model.model_type)
              ? model.model_type
              : model.model_type
                ? [model.model_type as string]
                : [];
            const ret = await addInstanceModel({
              provider_name: payload.llmFactory,
              instance_name: payload.instanceName,
              model_name: model.model_name,
              model_type: modelType,
              max_tokens: model.max_tokens ?? 0,
              ...(model.extra ? { extra: model.extra } : {}),
            });
            if (ret.code !== 0) {
              return ret;
            }
          }
        } else if (payload.formValues) {
          // Non-LIST_MODEL_PROVIDERS: add/update the single model
          // described by the form values.
          const fv = payload.formValues;
          const modelType = Array.isArray(fv.model_type)
            ? fv.model_type
            : fv.model_type
              ? [fv.model_type as string]
              : [];
          const ret = await addInstanceModel({
            provider_name: payload.llmFactory,
            instance_name: payload.instanceName,
            model_name: fv.model_name,
            model_type: modelType,
            max_tokens: fv.max_tokens ?? 0,
            ...(fv.is_tools !== undefined
              ? { extra: { is_tools: !!fv.is_tools } }
              : {}),
          });
          if (ret.code !== 0) {
            return ret;
          }
        }
        setProviderVisible(false);
      } finally {
        setProviderLoading(false);
      }
    },
    [addInstanceModel],
  );

  // Closing the modal also clears the viewMode flag so the next open
  // starts in the default (add) mode.
  const hideProviderModal = useCallback(() => {
    setProviderVisible(false);
    setViewMode(false);
  }, []);

  const { onApiKeyVerifying: onSoMarkVerifying } = useVerifySettings({
    onVerify: onSoMarkOk,
  });

  return (
    <div className="flex w-full border-[0.5px] border-border-button rounded-lg relative ">
      <Spotlight />
      <section className="flex flex-col gap-4 w-3/5 px-5 border-r-[0.5px] border-border-button overflow-auto scrollbar-auto">
        <SystemSetting />
        <UsedModel
          handleAddModel={handleAddModel}
          onEditInstance={handleEditInstance}
        />
      </section>
      <section className="flex flex-col w-2/5 overflow-auto scrollbar-auto">
        <AvailableModels handleAddModel={handleAddModel} />
      </section>
      {/* Unified ProviderModal (replaces 9 independent modals) */}
      <ProviderModal
        visible={providerVisible}
        hideModal={hideProviderModal}
        llmFactory={currentLlmFactory}
        loading={providerLoading}
        viewMode={viewMode}
        initialValues={viewModeInitialValues}
        baseUrlOptions={currentBaseUrlOptions as any}
        onOk={handleProviderOk}
        onVerify={handleProviderVerify}
        onViewModeOk={handleViewModeOk}
      />
      <BedrockModal
        visible={bedrockAddingVisible}
        hideModal={hideBedrockAddingModal}
        onOk={onBedrockAddingOk}
        loading={bedrockAddingLoading}
        llmFactory={LLMFactory.Bedrock}
        onVerify={(payload) => onBedrockAddingOk(payload, true)}
      ></BedrockModal>
      <SoMarkModal
        visible={somarkVisible}
        hideModal={hideSoMarkModal}
        onOk={onSoMarkOk}
        loading={somarkLoading}
        onVerify={onSoMarkVerifying}
      ></SoMarkModal>
    </div>
  );
};

export default ModelProviders;
