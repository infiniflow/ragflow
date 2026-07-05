import { LLMFactory } from '@/constants/llm';
import { useSetModalState } from '@/hooks/common-hooks';
import {
  useAddInstanceModel,
  useAddProviderInstance,
  useFetchAddedProviders,
  useFetchProviderInstances,
  useVerifyProviderConnection,
} from '@/hooks/use-llm-request';
import {
  IAddProviderInstanceRequestBody,
  IModelInfo,
} from '@/interfaces/request/llm';
import { useCallback, useMemo, useState } from 'react';
import { splitProviderPayload } from './payload-utils';

export type VerifyResult = {
  isValid: boolean | null;
  logs: string;
};

/**
 * Unified Provider instance submission hook
 * Internally handles both verify and save modes
 */
const useSubmitProviderInstance = () => {
  const { addProviderInstance } = useAddProviderInstance();
  const { addInstanceModel } = useAddInstanceModel();

  return useCallback(
    async (payload: IAddProviderInstanceRequestBody, isVerify = false) => {
      if (isVerify) {
        return addProviderInstance({ ...payload, verify: true });
      }

      // Multi-model flow: when model_info is provided as an array, the
      // backend is expected to create the instance and all listed models
      // in a single addProviderInstance call. Skip the instance/model split.
      if (Array.isArray((payload as any).model_info)) {
        return addProviderInstance(payload as IAddProviderInstanceRequestBody);
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

export const useVerifyConnection = () => {
  const { verifyProviderConnection } = useVerifyProviderConnection();

  return useCallback(
    async (
      providerName: string,
      apiKey: string,
      baseUrl?: string,
      region?: string,
      modelInfo?: IModelInfo[],
    ) => {
      const ret = await verifyProviderConnection({
        provider_name: providerName,
        api_key: apiKey,
        base_url: baseUrl,
        region: region,
        model_info: modelInfo,
      });

      if (ret.code === 0) {
        return {
          isValid: true,
          logs: ret.message,
        } as VerifyResult;
      } else {
        return {
          isValid: false,
          logs: ret.message,
        } as VerifyResult;
      }
    },
    [verifyProviderConnection],
  );
};

// ============ Hooks for retained special modals ============
// Bedrock and SoMark still use custom modal components.

export const useSubmitBedrock = () => {
  const [saveLoading, setSaveLoading] = useState(false);
  const submitProviderInstance = useSubmitProviderInstance();
  const verifyConnection = useVerifyConnection();
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
      const { instancePayload, modelPayload } = splitProviderPayload(payload);
      if (isVerify) {
        return verifyConnection(
          payload.llm_factory as string,
          JSON.stringify(instancePayload.api_key),
          instancePayload.base_url,
          instancePayload.region,
          [modelPayload],
        );
      }
      const ret = await submitProviderInstance(
        {
          ...instancePayload,
          max_tokens: modelPayload.max_tokens,
          model_info: [modelPayload],
        },
        false,
      );
      setSaveLoading(false);
      if (ret.code === 0) {
        hideBedrockAddingModal();
      }
    },
    [
      hideBedrockAddingModal,
      submitProviderInstance,
      setSaveLoading,
      verifyConnection,
    ],
  );

  return {
    bedrockAddingLoading: saveLoading,
    onBedrockAddingOk,
    bedrockAddingVisible,
    hideBedrockAddingModal,
    showBedrockAddingModal,
  };
};

export const useSubmitSoMark = () => {
  const [saveLoading, setSaveLoading] = useState(false);
  const submitProviderInstance = useSubmitProviderInstance();
  const verifyConnection = useVerifyConnection();
  const {
    visible: somarkVisible,
    hideModal: hideSoMarkModal,
    showModal: showSoMarkModal,
  } = useSetModalState();

  const onSoMarkOk = useCallback(
    async (payload: any, isVerify = false) => {
      if (!isVerify) {
        setSaveLoading(true);
      }
      const req = {
        instance_name: payload.instance_name,
        llm_factory: LLMFactory.SoMark,
        api_key: payload.somark_api_key || '',
        base_url: payload.somark_base_url,
        max_tokens: 0,
        model_info: [
          {
            model_name: payload.llm_name,
            model_type: ['ocr'],
            max_tokens: 0,
            extra: {
              somark_image_format: payload.somark_image_format,
              somark_formula_format: payload.somark_formula_format,
              somark_table_format: payload.somark_table_format,
              somark_cs_format: payload.somark_cs_format,
              somark_enable_text_cross_page:
                payload.somark_enable_text_cross_page,
              somark_enable_table_cross_page:
                payload.somark_enable_table_cross_page,
              somark_enable_title_level_recognition:
                payload.somark_enable_title_level_recognition,
              somark_enable_inline_image: payload.somark_enable_inline_image,
              somark_enable_table_image: payload.somark_enable_table_image,
              somark_enable_image_understanding:
                payload.somark_enable_image_understanding,
              somark_keep_header_footer: payload.somark_keep_header_footer,
            },
          },
        ],
      };
      try {
        if (isVerify) {
          return verifyConnection(
            LLMFactory.SoMark,
            req.api_key,
            req.base_url,
            undefined,
            req.model_info as IModelInfo[],
          );
        }
        const ret = await submitProviderInstance(
          req as IAddProviderInstanceRequestBody,
          false,
        );
        if (ret.code === 0) {
          hideSoMarkModal();
          return true;
        }
        return false;
      } finally {
        if (!isVerify) {
          setSaveLoading(false);
        }
      }
    },
    [submitProviderInstance, hideSoMarkModal, setSaveLoading, verifyConnection],
  );

  return {
    somarkVisible,
    hideSoMarkModal,
    showSoMarkModal,
    onSoMarkOk,
    somarkLoading: saveLoading,
  };
};

/**
 * Wraps the verify callback: provides a unified call with isVerify=true for the Verify button
 */
export const useVerifySettings = ({
  onVerify,
}: {
  onVerify: (postBody: any, isVerify?: boolean) => Promise<any>;
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
