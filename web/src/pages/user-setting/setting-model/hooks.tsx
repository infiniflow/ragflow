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
      if (isVerify) {
        const legacyPayload = payload as any;
        const modelType = Array.isArray(legacyPayload.model_type)
          ? (legacyPayload.model_type as string[])
          : legacyPayload.model_type
            ? [legacyPayload.model_type as string]
            : [];
        const apiKey = JSON.stringify({
          auth_mode: legacyPayload.auth_mode,
          bedrock_ak: legacyPayload.bedrock_ak,
          bedrock_sk: legacyPayload.bedrock_sk,
          aws_role_arn: legacyPayload.aws_role_arn,
        });
        return verifyConnection(
          payload.llm_factory as string,
          apiKey,
          legacyPayload.bedrock_region,
          undefined,
          [
            {
              model_type: modelType,
              model_name: (legacyPayload.model_name as string) ?? '',
              max_tokens: (legacyPayload.max_tokens as number) ?? 0,
            },
          ],
        );
      }
      const ret = await submitProviderInstance(payload, false);
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
        llm_name: payload.llm_name,
        model_type: 'ocr',
        api_key: payload.somark_api_key || '',
        api_base: payload.somark_base_url,
        max_tokens: 0,
        somark_image_format: payload.somark_image_format,
        somark_formula_format: payload.somark_formula_format,
        somark_table_format: payload.somark_table_format,
        somark_cs_format: payload.somark_cs_format,
        somark_enable_text_cross_page: payload.somark_enable_text_cross_page,
        somark_enable_table_cross_page: payload.somark_enable_table_cross_page,
        somark_enable_title_level_recognition:
          payload.somark_enable_title_level_recognition,
        somark_enable_inline_image: payload.somark_enable_inline_image,
        somark_enable_table_image: payload.somark_enable_table_image,
        somark_enable_image_understanding:
          payload.somark_enable_image_understanding,
        somark_keep_header_footer: payload.somark_keep_header_footer,
      };
      try {
        const ret = await submitProviderInstance(
          req as IAddProviderInstanceRequestBody,
          isVerify,
        );
        if (isVerify) {
          return {
            isValid: !!ret.data?.success,
            logs: ret.data?.message,
          } as VerifyResult;
        }
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
    [submitProviderInstance, hideSoMarkModal, setSaveLoading],
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
