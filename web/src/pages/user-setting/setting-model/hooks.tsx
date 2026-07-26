/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

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
// Bedrock and SoMark have been migrated to inline instance cards
// (BedrockInstanceCard / SoMarkInstanceCard); these legacy modal
// hooks are kept only for backward-compat references.

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
