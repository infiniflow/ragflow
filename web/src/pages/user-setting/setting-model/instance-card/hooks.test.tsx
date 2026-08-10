/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 */

jest.mock('@/hooks/use-llm-request', () => ({
  useDeleteProviderInstance: jest.fn(),
  useFetchAvailableProviders: jest.fn(),
  useFetchProviderInstance: jest.fn(),
  useVerifyProviderConnection: jest.fn(),
}));
jest.mock('../provider-schema/hooks', () => ({
  useProviderFields: jest.fn(),
}));

import { useVerifyProviderConnection } from '@/hooks/use-llm-request';
import { act, renderHook } from '@testing-library/react';
import { useInstanceSaveState, useVerifyProvider } from './hooks';

describe('useInstanceSaveState acknowledgements', () => {
  const model = (name: string, maxTokens: number) => ({
    model_name: name,
    model_type: ['chat'],
    max_tokens: maxTokens,
  });

  it('derives the submit region from an input-select base URL', () => {
    const intlURL = 'https://api.siliconflow.com/v1';
    const values = { api_key: 'token', base_url: intlURL };
    const formRef = {
      current: { getValues: () => values },
    } as any;
    const modelInfoRef = { current: [model('model-a', 1024)] };

    const { result } = renderHook(() =>
      useInstanceSaveState({
        formRef,
        providerName: 'SiliconFlow',
        instanceName: '',
        instanceId: undefined,
        isDraft: true,
        draftName: 'intl-instance',
        instanceDetails: undefined,
        initialValues: values,
        modelInfoRef,
        baseUrlRegionMaps: {
          base_url: new Map([[intlURL, 'intl']]),
        },
      }),
    );

    expect(result.current.getSavePayload()?.payload.region).toBe('intl');
  });

  it('derives the verify region from an input-select base URL', async () => {
    const intlURL = 'https://api.siliconflow.com/v1';
    const verifyProviderConnection = jest
      .fn()
      .mockResolvedValue({ code: 0, message: 'success' });
    (useVerifyProviderConnection as jest.Mock).mockReturnValue({
      verifyProviderConnection,
    });
    const formRef = {
      current: {
        getValues: () => ({ api_key: 'token', base_url: intlURL }),
      },
    } as any;

    const { result } = renderHook(() =>
      useVerifyProvider('SiliconFlow', formRef, undefined, {
        base_url: new Map([[intlURL, 'intl']]),
      }),
    );
    await act(async () => {
      await result.current({});
    });

    expect(verifyProviderConnection).toHaveBeenCalledWith(
      expect.objectContaining({ region: 'intl' }),
    );
  });

  it('does not absorb an edit made after the saved request snapshot', () => {
    let values = { api_key: 'persisted', base_url: '', region: 'default' };
    const formRef = {
      current: { getValues: () => values },
    } as any;
    const modelInfoRef = { current: [model('model-a', 1024)] };

    const { result } = renderHook(() =>
      useInstanceSaveState({
        formRef,
        providerName: 'OpenAI',
        instanceName: 'instance',
        instanceId: 'instance-id',
        isDraft: false,
        draftName: '',
        instanceDetails: { id: 'instance-id' } as any,
        initialValues: values,
        modelInfoRef,
      }),
    );

    values = { ...values, api_key: 'sent-value' };
    const sent = result.current.getSavePayload();
    expect(sent).not.toBeNull();

    values = { ...values, api_key: 'edited-in-flight' };
    act(() => result.current.markSaved(sent!.payload));

    expect(result.current.getSavePayload()?.payload.api_key).toBe(
      'edited-in-flight',
    );
  });

  it('patches only acknowledged model state into the baseline', () => {
    let values = { api_key: 'persisted', base_url: '', region: 'default' };
    const formRef = {
      current: { getValues: () => values },
    } as any;
    const modelInfoRef = { current: [model('model-a', 1024)] };

    const { result } = renderHook(() =>
      useInstanceSaveState({
        formRef,
        providerName: 'OpenAI',
        instanceName: 'instance',
        instanceId: 'instance-id',
        isDraft: false,
        draftName: '',
        instanceDetails: { id: 'instance-id' } as any,
        initialValues: values,
        modelInfoRef,
      }),
    );

    const savedPayload = {
      ...result.current.getSavePayload()!.payload,
      api_key: 'persisted',
      model_info: [model('model-a', 1024)],
    };
    act(() => result.current.markSaved(savedPayload));

    values = { ...values, api_key: 'unsaved-credential' };
    const patchedModels = [model('model-a', 2048)];
    modelInfoRef.current = patchedModels;
    act(() => result.current.markModelsEdited(patchedModels));

    const pending = result.current.getSavePayload();
    expect(pending?.payload.api_key).toBe('unsaved-credential');
    expect(pending?.payload.model_info).toEqual(patchedModels);
  });

  it('preserves acknowledged models when refreshed details arrive later', () => {
    let values = { api_key: 'persisted', base_url: '', region: 'default' };
    const formRef = {
      current: { getValues: () => values },
    } as any;
    const models = [model('model-a', 1024)];
    const modelInfoRef = { current: models };
    const initialDetails = { id: 'instance-id', api_key: 'persisted' };

    const { result, rerender } = renderHook(
      ({ initialValues, instanceDetails }) =>
        useInstanceSaveState({
          formRef,
          providerName: 'OpenAI',
          instanceName: 'instance',
          instanceId: 'instance-id',
          isDraft: false,
          draftName: '',
          instanceDetails: instanceDetails as any,
          initialValues,
          modelInfoRef,
        }),
      {
        initialProps: {
          initialValues: values,
          instanceDetails: initialDetails,
        },
      },
    );

    values = { ...values, api_key: 'saved' };
    const savedPayload = {
      ...result.current.getSavePayload()!.payload,
      model_info: models,
    };
    act(() => result.current.markSaved(savedPayload));

    rerender({
      initialValues: { ...values },
      instanceDetails: { id: 'instance-id', api_key: 'saved' },
    });

    expect(result.current.getSavePayload()).toBeNull();
  });

  it('re-seeds when provider defaults change the computed remote values', () => {
    let values = { api_key: 'persisted', base_url: '', region: 'default' };
    const formRef = {
      current: { getValues: () => values },
    } as any;
    const modelInfoRef = { current: [] };
    const instanceDetails = { id: 'instance-id', api_key: 'persisted' };

    const { result, rerender } = renderHook(
      ({ initialValues }) =>
        useInstanceSaveState({
          formRef,
          providerName: 'OpenAI',
          instanceName: 'instance',
          instanceId: 'instance-id',
          isDraft: false,
          draftName: '',
          instanceDetails: instanceDetails as any,
          initialValues,
          modelInfoRef,
        }),
      { initialProps: { initialValues: values } },
    );

    values = {
      ...values,
      base_url: 'https://provider-default.example.com',
    };
    rerender({ initialValues: values });

    expect(result.current.getSavePayload()).toBeNull();
  });

  it('uses a top-level submit transform for both baseline and live payloads', () => {
    let values = {
      google_project_id: 'project-a',
      google_region: 'us-central1',
      google_service_account_key: 'service-account-a',
    };
    const formRef = {
      current: { getValues: () => values },
    } as any;
    const modelInfoRef = { current: [] };
    const submitTransform = (formValues: Record<string, any>) => ({
      instance_name: formValues.instance_name,
      llm_factory: 'GoogleCloud',
      google_project_id: formValues.google_project_id,
      google_region: formValues.google_region,
      google_service_account_key: formValues.google_service_account_key,
      model_info: [],
    });

    const { result } = renderHook(() =>
      useInstanceSaveState({
        formRef,
        providerName: 'GoogleCloud',
        instanceName: 'instance',
        instanceId: 'instance-id',
        isDraft: false,
        draftName: '',
        instanceDetails: { id: 'instance-id' } as any,
        initialValues: values,
        modelInfoRef,
        submitTransform,
      }),
    );

    expect(result.current.getSavePayload()).toBeNull();

    expect(
      result.current.buildInstanceUpdatePayload([model('model-a', 2048)]),
    ).toMatchObject({
      google_project_id: 'project-a',
      google_region: 'us-central1',
      google_service_account_key: 'service-account-a',
      model_info: [model('model-a', 2048)],
    });

    values = { ...values, google_project_id: 'project-b' };
    expect(result.current.getSavePayload()?.payload.google_project_id).toBe(
      'project-b',
    );
  });

  it('uses a nested submit transform for both baseline and live payloads', () => {
    let values = {
      opendataloader_apiserver: 'https://loader.example.com',
      opendataloader_api_key: 'loader-key-a',
    };
    const formRef = {
      current: { getValues: () => values },
    } as any;
    const modelInfoRef = { current: [] };
    const submitTransform = (formValues: Record<string, any>) => ({
      instance_name: formValues.instance_name,
      llm_factory: 'OpenDataLoader',
      api_key: {
        opendataloader_apiserver: formValues.opendataloader_apiserver,
        opendataloader_api_key: formValues.opendataloader_api_key,
      },
      base_url: '',
      model_info: [],
    });

    const { result } = renderHook(() =>
      useInstanceSaveState({
        formRef,
        providerName: 'OpenDataLoader',
        instanceName: 'instance',
        instanceId: 'instance-id',
        isDraft: false,
        draftName: '',
        instanceDetails: { id: 'instance-id' } as any,
        initialValues: values,
        modelInfoRef,
        submitTransform,
      }),
    );

    expect(result.current.getSavePayload()).toBeNull();

    values = { ...values, opendataloader_api_key: 'loader-key-b' };
    expect(result.current.getSavePayload()?.payload.api_key).toEqual({
      opendataloader_apiserver: 'https://loader.example.com',
      opendataloader_api_key: 'loader-key-b',
    });
  });
});
