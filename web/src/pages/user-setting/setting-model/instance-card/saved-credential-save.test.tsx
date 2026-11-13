import { renderHook } from '@testing-library/react';
import { useRef } from 'react';

import { LLMFactory } from '@/constants/llm';
import { LocalLlmConfigs } from '../provider-schema/field-config/local-llm-configs';
import { ProviderConfigMap } from '../provider-schema/field-config/provider-config-map';
import { SAVED_CREDENTIAL_MASK, useInstanceSaveState } from './hooks';

jest.mock('@/components/dynamic-form', () => ({
  FormFieldType: new Proxy({}, { get: (_target, key) => String(key) }),
}));

jest.mock('@/hooks/use-llm-request', () => ({}));
jest.mock('@/hooks/common-hooks', () => ({
  useTranslate: () => ({ t: (key: string) => key }),
}));
jest.mock('../provider-schema/hooks', () => ({ useProviderFields: jest.fn() }));

describe('saved provider credential updates', () => {
  it('does not treat an unchanged transformed instance as dirty', () => {
    const instance = {
      id: 'instance-id',
      instance_name: 'openai-compatible',
    };
    const initialValues = {
      api_key: SAVED_CREDENTIAL_MASK,
      credential_status: { api_key: true },
      base_url: 'https://example.com/v1',
      region: 'default',
    };
    const formValues = { ...initialValues };
    const config = LocalLlmConfigs[LLMFactory.OpenAiAPICompatible];

    const { result } = renderHook(() => {
      const formRef = useRef({ getValues: () => formValues } as any);
      const modelInfoRef = useRef<any[]>([]);
      return useInstanceSaveState({
        formRef,
        providerName: LLMFactory.OpenAiAPICompatible,
        instanceName: instance.instance_name,
        instanceId: instance.id,
        isDraft: false,
        draftName: '',
        instanceDetails: instance as any,
        initialValues,
        modelInfoRef,
        submitTransform: config.submitTransform,
      });
    });

    expect(result.current.getSavePayload()).toBeNull();
  });

  it('omits an empty API key when saving another field on a keyless instance', () => {
    const instance = {
      id: 'instance-id',
      instance_name: 'openai-compatible',
    };
    const initialValues = {
      api_key: '',
      credential_status: {},
      base_url: 'https://example.com/v1',
      region: 'default',
    };
    const formValues = { ...initialValues };
    const config = LocalLlmConfigs[LLMFactory.OpenAiAPICompatible];

    const { result } = renderHook(() => {
      const formRef = useRef({ getValues: () => formValues } as any);
      const modelInfoRef = useRef<any[]>([]);
      return useInstanceSaveState({
        formRef,
        providerName: LLMFactory.OpenAiAPICompatible,
        instanceName: instance.instance_name,
        instanceId: instance.id,
        isDraft: false,
        draftName: '',
        instanceDetails: instance as any,
        initialValues,
        modelInfoRef,
        submitTransform: config.submitTransform,
      });
    });

    formValues.base_url = 'https://updated.example.com/v1';
    const save = result.current.getSavePayload();
    expect(save?.payload.base_url).toBe('https://updated.example.com/v1');
    expect(save?.payload).not.toHaveProperty('api_key');
  });

  it('sends safe Google Cloud settings in the api_key config', () => {
    const instance = {
      id: 'instance-id',
      instance_name: 'google-cloud',
    };
    const initialValues = {
      google_project_id: 'project-1',
      google_region: 'us-central1',
      google_service_account_key: SAVED_CREDENTIAL_MASK,
      credential_status: { google_service_account_key: true },
      base_url: '',
      region: 'default',
    };
    const formValues = { ...initialValues };
    const config = ProviderConfigMap[LLMFactory.GoogleCloud];

    const { result } = renderHook(() => {
      const formRef = useRef({ getValues: () => formValues } as any);
      const modelInfoRef = useRef<any[]>([]);
      return useInstanceSaveState({
        formRef,
        providerName: LLMFactory.GoogleCloud,
        instanceName: instance.instance_name,
        instanceId: instance.id,
        isDraft: false,
        draftName: '',
        instanceDetails: instance as any,
        initialValues,
        modelInfoRef,
        submitTransform: config.submitTransform,
      });
    });

    expect(result.current.getSavePayload()).toBeNull();
    formValues.google_region = 'europe-west1';

    const save = result.current.getSavePayload();
    expect(save?.payload.api_key).toEqual({
      google_project_id: 'project-1',
      google_region: 'europe-west1',
      google_service_account_key: '',
    });
    expect(save?.payload).not.toHaveProperty('google_project_id');
    expect(save?.payload).not.toHaveProperty('google_region');
  });

  it('sends changed API keys and keeps later edits dirty', () => {
    const instance = {
      id: 'instance-id',
      instance_name: 'openai-compatible',
    };
    const initialValues = {
      api_key: SAVED_CREDENTIAL_MASK,
      api_key_configured: true,
      credential_status: { api_key: true },
      base_url: 'https://example.com/v1',
      region: 'default',
    };
    const formValues = { ...initialValues };
    const config = LocalLlmConfigs[LLMFactory.OpenAiAPICompatible];

    const { result } = renderHook(() => {
      const formRef = useRef({ getValues: () => formValues } as any);
      const modelInfoRef = useRef<any[]>([]);
      return useInstanceSaveState({
        formRef,
        providerName: LLMFactory.OpenAiAPICompatible,
        instanceName: instance.instance_name,
        instanceId: instance.id,
        isDraft: false,
        draftName: '',
        instanceDetails: instance as any,
        initialValues,
        modelInfoRef,
        submitTransform: config.submitTransform,
      });
    });

    formValues.api_key = 'first-new-key';
    const firstSave = result.current.getSavePayload();
    expect(firstSave?.payload.api_key).toBe('first-new-key');

    result.current.markSaved();
    formValues.api_key = 'second-new-key';
    const secondSave = result.current.getSavePayload();
    expect(secondSave?.payload.api_key).toBe('second-new-key');
  });

  it.each([
    {
      name: 'an unchanged saved credential',
      api_key: SAVED_CREDENTIAL_MASK,
      credential_status: { api_key: true },
    },
    {
      name: 'an instance with no credential',
      api_key: '',
      credential_status: {},
    },
  ])('omits api_key for $name', ({ api_key, credential_status }) => {
    const instance = {
      id: 'instance-id',
      instance_name: 'openai-compatible',
    };
    const initialValues = {
      api_key,
      credential_status,
      base_url: 'https://example.com/v1',
      region: 'default',
    };
    const formValues = { ...initialValues };
    const config = LocalLlmConfigs[LLMFactory.OpenAiAPICompatible];

    const { result } = renderHook(() => {
      const formRef = useRef({ getValues: () => formValues } as any);
      const modelInfoRef = useRef<any[]>([]);
      return useInstanceSaveState({
        formRef,
        providerName: LLMFactory.OpenAiAPICompatible,
        instanceName: instance.instance_name,
        instanceId: instance.id,
        isDraft: false,
        draftName: '',
        instanceDetails: instance as any,
        initialValues,
        modelInfoRef,
        submitTransform: config.submitTransform,
      });
    });

    expect(result.current.buildPayload()).not.toHaveProperty('api_key');
  });

  it('sends an explicit clear only when a saved credential is emptied', () => {
    const instance = {
      id: 'instance-id',
      instance_name: 'openai-compatible',
    };
    const initialValues = {
      api_key: SAVED_CREDENTIAL_MASK,
      credential_status: { api_key: true },
      base_url: 'https://example.com/v1',
      region: 'default',
    };
    const formValues = { ...initialValues, api_key: '' };
    const config = LocalLlmConfigs[LLMFactory.OpenAiAPICompatible];

    const { result } = renderHook(() => {
      const formRef = useRef({ getValues: () => formValues } as any);
      const modelInfoRef = useRef<any[]>([]);
      return useInstanceSaveState({
        formRef,
        providerName: LLMFactory.OpenAiAPICompatible,
        instanceName: instance.instance_name,
        instanceId: instance.id,
        isDraft: false,
        draftName: '',
        instanceDetails: instance as any,
        initialValues,
        modelInfoRef,
        submitTransform: config.submitTransform,
      });
    });

    expect(result.current.buildPayload()?.api_key).toEqual({ api_key: null });
  });
});
