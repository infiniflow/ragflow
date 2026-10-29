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

import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react';
import { createRef } from 'react';

import { BedrockInstanceCard } from './bedrock-instance-card';
import {
  applySavedCredentialEdits,
  buildApiKeyValue,
  SAVED_CREDENTIAL_MASK,
} from './hooks';

const originalResizeObserver = global.ResizeObserver;
beforeAll(() => {
  global.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
});
afterAll(() => {
  global.ResizeObserver = originalResizeObserver;
});

type TestCardRef = {
  validate: () => Promise<boolean>;
  getSavePayload: () => {
    apiKind: 'add' | 'update';
    instanceName: string;
    isDraft: boolean;
    payload: Record<string, unknown>;
  } | null;
  markSaved: () => void;
};

const mockCatalogCredentials = jest.fn();
const mockFetchProviderInstance = jest.fn();
const mockRefetchProviderInstance = jest.fn();
type MockModelInfo = {
  model_name: string;
  model_type: string[];
  max_tokens: number;
};
let mockDiscoveredModels: MockModelInfo[];
let mockInstanceModelsLoaded: boolean;
const savedInstanceDetails = {
  id: 'instance-id',
  instance_name: 'saved-instance',
  provider_id: 'provider-id',
  region: 'us-east-1',
  status: 'active',
  is_owner: true,
  api_key_configured: true,
  api_key_config: {
    auth_mode: 'bedrock_api_key',
    bedrock_region: 'us-east-1',
  },
  credential_status: { bedrock_api_key: true },
};

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/hooks/common-hooks', () => ({
  useTranslate: () => ({ t: (key: string) => key }),
}));

jest.mock('@/hooks/logic-hooks/use-build-options', () => ({
  useBuildModelTypeOptions: () => ({ buildModelTypeOptions: jest.fn() }),
}));

jest.mock('@/hooks/use-llm-request', () => {
  return {
    useDeleteProviderInstance: () => ({ deleteProviderInstance: jest.fn() }),
    useFetchProviderInstance: (...args: unknown[]) =>
      mockFetchProviderInstance(...args),
    useVerifyProviderConnection: () => ({
      verifyProviderConnection: jest.fn(),
    }),
  };
});

jest.mock('@/components/confirm-delete-dialog', () => ({
  ConfirmDeleteDialog: ({ children }: { children: React.ReactNode }) =>
    children,
}));

jest.mock('@/components/ui/button', () => ({
  Button: ({
    children,
    ...props
  }: React.ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button {...props}>{children}</button>
  ),
}));

jest.mock('@/components/originui/select-with-search', () => ({
  SelectWithSearch: () => null,
}));

jest.mock('@/components/ui/multi-select', () => ({ MultiSelect: () => null }));
jest.mock('@/components/ui/segmented', () => ({
  Segmented: ({ onChange }: { onChange: (value: string) => void }) => (
    <>
      <button type="button" onClick={() => onChange('access_key_secret')}>
        accessKey
      </button>
      <button type="button" onClick={() => onChange('bedrock_api_key')}>
        apiKey
      </button>
    </>
  ),
}));

jest.mock('./models-section', () => {
  const React = jest.requireActual<typeof import('react')>('react');
  return {
    ModelsSection: (props: {
      getFormValues: () => Record<string, unknown>;
      verifyTransform: (values: Record<string, unknown>) => unknown;
      onInstanceModelsChange?: (models: MockModelInfo[]) => void;
      onInstanceModelsEdited?: () => void;
      onInstanceModelsStatusChange?: (ready: boolean) => void;
    }) => {
      React.useEffect(() => {
        mockCatalogCredentials(props.verifyTransform(props.getFormValues()));
        props.onInstanceModelsChange?.(mockDiscoveredModels);
        if (mockInstanceModelsLoaded) {
          props.onInstanceModelsEdited?.();
        }
        // ModelsSection resolves credentials from its mount effect.
        // oxlint-disable-next-line react/exhaustive-deps
      }, []);
      React.useEffect(() => {
        props.onInstanceModelsStatusChange?.(mockInstanceModelsLoaded);
      });
      return null;
    },
  };
});
jest.mock('./verify-button', () => ({ __esModule: true, default: () => null }));

describe('BedrockInstanceCard', () => {
  beforeEach(() => {
    mockCatalogCredentials.mockReset();
    mockFetchProviderInstance
      .mockReset()
      .mockImplementation((providerName) => ({
        data: providerName ? savedInstanceDetails : undefined,
        refetch: mockRefetchProviderInstance,
      }));
    mockRefetchProviderInstance.mockReset();
    mockInstanceModelsLoaded = true;
    mockDiscoveredModels = [
      {
        model_name: 'amazon.nova-lite-v1:0',
        model_type: ['chat', 'vision'],
        max_tokens: 8192,
      },
    ];
  });

  it('drops unchanged saved masks when another Bedrock credential changes', () => {
    expect(
      applySavedCredentialEdits(
        {
          auth_mode: 'iam_role',
          aws_role_arn: SAVED_CREDENTIAL_MASK,
          bedrock_ak: SAVED_CREDENTIAL_MASK,
          bedrock_sk: SAVED_CREDENTIAL_MASK,
        },
        {
          aws_role_arn: 'arn:aws:iam::123456789012:role/model',
          bedrock_ak: SAVED_CREDENTIAL_MASK,
          bedrock_sk: SAVED_CREDENTIAL_MASK,
        },
        { aws_role_arn: true, bedrock_ak: true, bedrock_sk: true },
      ),
    ).toEqual({
      auth_mode: 'iam_role',
      aws_role_arn: 'arn:aws:iam::123456789012:role/model',
    });
  });

  it('omits unchanged masks and sends null when a saved credential is cleared', () => {
    expect(
      buildApiKeyValue({ api_key: SAVED_CREDENTIAL_MASK }, { api_key: true }),
    ).toBeUndefined();
    expect(buildApiKeyValue({ api_key: '' }, { api_key: true })).toEqual({
      api_key: null,
    });
    expect(
      applySavedCredentialEdits(
        { auth_mode: 'bedrock_api_key', bedrock_api_key: '' },
        { bedrock_api_key: '' },
        { bedrock_api_key: true },
      ),
    ).toEqual({ auth_mode: 'bedrock_api_key', bedrock_api_key: null });
  });

  it('creates an API key instance without manual model fields', async () => {
    const ref = createRef<TestCardRef>();
    render(
      <BedrockInstanceCard
        ref={ref}
        providerName="Bedrock"
        isDraft
        instance={{
          id: '',
          instance_name: '',
          provider_id: 'provider-id',
          region: 'us-east-1',
          status: 'active',
        }}
      />,
    );

    expect(screen.getByText('modelType')).toBeTruthy();
    expect(screen.getByPlaceholderText('bedrockModelNameMessage')).toBeTruthy();
    expect(screen.getByPlaceholderText('maxTokensTip')).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: 'apiKey' }));

    expect(screen.queryByText('modelType')).toBeNull();
    expect(screen.queryByPlaceholderText('bedrockModelNameMessage')).toBeNull();
    expect(screen.queryByPlaceholderText('maxTokensTip')).toBeNull();

    fireEvent.change(screen.getByTestId('instance-name-input'), {
      target: { value: 'api-key-instance' },
    });
    fireEvent.change(screen.getByPlaceholderText('apiKeyMessage'), {
      target: { value: 'new-key' },
    });

    await waitFor(async () => expect(await ref.current?.validate()).toBe(true));

    const result = ref.current?.getSavePayload();
    expect(result).toMatchObject({
      apiKind: 'add',
      instanceName: 'api-key-instance',
      isDraft: true,
      payload: {
        instance_name: 'api-key-instance',
        llm_factory: 'Bedrock',
        api_key: {
          auth_mode: 'bedrock_api_key',
          bedrock_api_key: 'new-key',
          bedrock_region: 'us-east-1',
        },
      },
    });
    expect(result?.payload).toMatchObject({ model_info: mockDiscoveredModels });
    expect(result?.payload).not.toHaveProperty('max_tokens');
  });

  it('requires a discovered model before saving an API key instance', async () => {
    mockDiscoveredModels = [];
    const ref = createRef<TestCardRef>();
    render(
      <BedrockInstanceCard
        ref={ref}
        providerName="Bedrock"
        isDraft
        instance={{
          id: '',
          instance_name: '',
          provider_id: 'provider-id',
          region: 'us-east-1',
          status: 'active',
        }}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'apiKey' }));
    fireEvent.change(screen.getByTestId('instance-name-input'), {
      target: { value: 'api-key-instance' },
    });
    fireEvent.change(screen.getByPlaceholderText('apiKeyMessage'), {
      target: { value: 'new-key' },
    });

    await waitFor(async () =>
      expect(await ref.current?.validate()).toBe(false),
    );
  });

  it('updates saved credentials with the current model list', async () => {
    const ref = createRef<TestCardRef>();
    render(
      <BedrockInstanceCard
        ref={ref}
        providerName="Bedrock"
        defaultOpen
        instance={{
          id: 'instance-id',
          instance_name: 'saved-instance',
          provider_id: 'provider-id',
          region: 'us-east-1',
          status: 'active',
        }}
      />,
    );

    const credentialInput = await waitFor(() => {
      const input = document.querySelector<HTMLInputElement>(
        'input[name="bedrock_api_key"]',
      );
      expect(input).not.toBeNull();
      return input!;
    });
    expect(credentialInput.value).toBe(SAVED_CREDENTIAL_MASK);
    await waitFor(() => expect(ref.current?.getSavePayload()).toBeNull());
    const input = document.querySelector<HTMLInputElement>(
      'input[name="bedrock_api_key"]',
    );
    expect(input?.value).toBe(SAVED_CREDENTIAL_MASK);
    fireEvent.change(input!, { target: { value: 'new-key' } });

    await waitFor(() => {
      const result = ref.current?.getSavePayload();
      expect(result).toMatchObject({
        apiKind: 'update',
        instanceName: 'saved-instance',
        isDraft: false,
        payload: {
          provider_name: 'Bedrock',
          id: 'instance-id',
          instance_name: 'saved-instance',
          verify: false,
          api_key: {
            auth_mode: 'bedrock_api_key',
            bedrock_api_key: 'new-key',
            bedrock_region: 'us-east-1',
          },
          model_info: mockDiscoveredModels,
        },
      });
    });
  });

  it('clearing a masked field removes the saved credential on save', async () => {
    const ref = createRef<TestCardRef>();
    render(
      <BedrockInstanceCard
        ref={ref}
        providerName="Bedrock"
        defaultOpen
        instance={{
          id: 'instance-id',
          instance_name: 'saved-instance',
          provider_id: 'provider-id',
          region: 'us-east-1',
          status: 'active',
        }}
      />,
    );

    const input = await waitFor(() => {
      const field = document.querySelector<HTMLInputElement>(
        'input[name="bedrock_api_key"]',
      );
      expect(field).not.toBeNull();
      return field!;
    });
    expect(input.value).toBe(SAVED_CREDENTIAL_MASK);
    fireEvent.change(input, { target: { value: '' } });

    await waitFor(() => {
      const payload = ref.current?.getSavePayload()?.payload;
      expect(payload?.clear_api_key).toBeUndefined();
      expect(payload?.api_key).toMatchObject({ bedrock_api_key: null });
    });
    expect(
      screen.queryByRole('button', { name: /removeSavedCredentials/i }),
    ).toBeNull();
    let isValid = false;
    await act(async () => {
      isValid = (await ref.current?.validate()) ?? false;
    });
    expect(isValid).toBe(true);
    act(() => ref.current?.markSaved());
    await waitFor(() => {
      expect(ref.current?.getSavePayload()).toBeNull();
    });
  });

  it('stops saving when the model snapshot becomes stale', async () => {
    const ref = createRef<TestCardRef>();
    const card = (
      <BedrockInstanceCard
        ref={ref}
        providerName="Bedrock"
        defaultOpen
        instance={{
          id: 'instance-id',
          instance_name: 'saved-instance',
          provider_id: 'provider-id',
          region: 'us-east-1',
          status: 'active',
        }}
      />
    );
    const { rerender } = render(card);

    await waitFor(async () => expect(await ref.current?.validate()).toBe(true));

    mockInstanceModelsLoaded = false;
    rerender(card);

    await waitFor(async () =>
      expect(await ref.current?.validate()).toBe(false),
    );
  });

  it('restores safe auth metadata without exposing credentials before model discovery starts', async () => {
    mockFetchProviderInstance.mockReturnValue({
      data: undefined,
      refetch: mockRefetchProviderInstance,
    });
    const { rerender } = render(
      <BedrockInstanceCard
        providerName="Bedrock"
        defaultOpen
        instance={{
          id: 'instance-id',
          instance_name: 'saved-instance',
          provider_id: 'provider-id',
          region: 'us-east-1',
          status: 'active',
        }}
      />,
    );

    expect(mockCatalogCredentials).not.toHaveBeenCalled();

    mockFetchProviderInstance.mockReturnValue({
      data: savedInstanceDetails,
      refetch: mockRefetchProviderInstance,
    });
    rerender(
      <BedrockInstanceCard
        providerName="Bedrock"
        defaultOpen
        instance={{
          id: 'instance-id',
          instance_name: 'saved-instance',
          provider_id: 'provider-id',
          region: 'us-east-1',
          status: 'active',
        }}
      />,
    );

    await waitFor(() =>
      expect(mockCatalogCredentials).toHaveBeenCalledWith({
        apiKey: {
          auth_mode: 'bedrock_api_key',
          bedrock_region: 'us-east-1',
        },
        baseUrl: undefined,
        region: 'default',
      }),
    );
  });
});
