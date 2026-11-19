import { renderHook } from '@testing-library/react';
import { useRef } from 'react';

import { LLMFactory } from '@/constants/llm';
import { LocalLlmConfigs } from '../provider-schema/field-config/local-llm-configs';
import {
  SAVED_CREDENTIAL_MASK,
  unwrapApiKey,
  useInstanceSaveState,
} from './hooks';
import { pickSavedProviderConfigOverrides } from './saved-provider-config';

jest.mock('@/components/dynamic-form', () => ({
  FormFieldType: new Proxy({}, { get: (_target, key) => String(key) }),
}));
jest.mock('@/hooks/use-llm-request', () => ({}));
jest.mock('@/hooks/common-hooks', () => ({
  useTranslate: () => ({ t: (key: string) => key }),
}));
jest.mock('../provider-schema/hooks', () => ({ useProviderFields: jest.fn() }));

describe('saved NovaCode instance', () => {
  it('restores protocol and effort as non-secret form values', () => {
    const settings = { wire_api: 'responses', reasoning_effort: 'high' };
    expect(unwrapApiKey(settings).nested).toEqual(settings);
    expect(pickSavedProviderConfigOverrides(settings)).toEqual(settings);
  });

  it('persists effort edits without resending the masked API key', () => {
    const initialValues = {
      api_key: SAVED_CREDENTIAL_MASK,
      credential_status: { api_key: true },
      base_url: 'https://ai.novacode.top/v1',
      wire_api: 'responses',
      reasoning_effort: 'medium',
    };
    const formValues = { ...initialValues };
    const instance = { id: 'nova-instance', instance_name: 'NovaCode' };
    const { result } = renderHook(() => {
      const formRef = useRef({ getValues: () => formValues } as any);
      const modelInfoRef = useRef<any[]>([]);
      return useInstanceSaveState({
        formRef,
        providerName: LLMFactory.NovaCode,
        instanceName: instance.instance_name,
        instanceId: instance.id,
        isDraft: false,
        draftName: '',
        instanceDetails: instance as any,
        initialValues,
        modelInfoRef,
        submitTransform: LocalLlmConfigs[LLMFactory.NovaCode].submitTransform,
      });
    });

    expect(result.current.getSavePayload()).toBeNull();
    formValues.reasoning_effort = 'high';
    expect(result.current.getSavePayload()?.payload.api_key).toEqual({
      wire_api: 'responses',
      reasoning_effort: 'high',
    });
  });
});
