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

let mockDraftSequence = 0;
const mockAddProviderInstance = jest.fn();
const mockUpdateProviderInstance = jest.fn();
const mockInvalidateQueries = jest.fn();
const mockMessageError = jest.fn();
const mockValidate = jest.fn();
const MockGetSavePayload = jest.fn();
let MockMutationCount = 0;

jest.mock('@/components/spotlight', () => () => null);
jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { error: (...args: unknown[]) => mockMessageError(...args) },
}));
jest.mock('@/hooks/common-hooks', () => ({
  useTranslate: () => ({ t: (key: string) => key }),
}));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/hooks/use-llm-request', () => ({
  LlmKeys: {
    providerInstances: (providerName: string) => [
      'providerInstances',
      providerName,
    ],
  },
  useAddProviderInstance: () => ({
    addProviderInstance: mockAddProviderInstance,
  }),
  useFetchAddedProviders: () => ({
    data: [{ name: 'OpenAI', has_instance: false }],
  }),
  useFetchProviderInstances: () => ({ data: [], loading: false }),
  useUpdateProviderInstance: () => ({
    updateProviderInstance: mockUpdateProviderInstance,
  }),
}));
jest.mock('@tanstack/react-query', () => ({
  useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  useIsMutating: () => MockMutationCount,
}));
jest.mock('./instance-card/provider-instance-card', () => {
  const React = jest.requireActual('react');
  return {
    ProviderInstanceCard: React.forwardRef((props: any, ref: any) => {
      const draftNumberRef = React.useRef(null) as {
        current: number | null;
      };
      if (props.isDraft && draftNumberRef.current === null) {
        mockDraftSequence += 1;
        draftNumberRef.current = mockDraftSequence;
      }
      const instanceName = `draft-${draftNumberRef.current}`;
      React.useImperativeHandle(ref, () => ({
        getSavePayload: () => MockGetSavePayload(instanceName),
        validate: () => mockValidate(instanceName),
        markSaved: jest.fn(),
      }));
      return React.createElement('div', {
        'data-testid': 'draft-card',
        'data-instance-name': instanceName,
      });
    }),
  };
});
jest.mock('./layout/provider-header-bar', () => ({
  ProviderHeaderBar: ({ onSave }: { onSave: () => void }) => {
    const React = jest.requireActual('react');
    return React.createElement(
      'button',
      { type: 'button', onClick: onSave, 'data-testid': 'save-all' },
      'save',
    );
  },
}));
jest.mock('./layout/sidebar', () => ({
  Sidebar: ({ onSelect }: { onSelect: (value: string) => void }) => {
    const React = jest.requireActual('react');
    const handleSelectOpenAI = () => onSelect('OpenAI');
    const handleSelectAnthropic = () => onSelect('Anthropic');
    return React.createElement(
      React.Fragment,
      null,
      React.createElement(
        'button',
        {
          type: 'button',
          onClick: handleSelectOpenAI,
          'data-testid': 'select-provider',
        },
        'OpenAI',
      ),
      React.createElement(
        'button',
        {
          type: 'button',
          onClick: handleSelectAnthropic,
          'data-testid': 'select-other-provider',
        },
        'Anthropic',
      ),
    );
  },
}));
jest.mock('./layout/system-setting', () => () => null);

import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react';
import React from 'react';
import SettingModelV2 from './index';

describe('SettingModelV2 batch save', () => {
  beforeAll(() => {
    (globalThis as any).React = React;
  });

  afterAll(() => {
    delete (globalThis as any).React;
  });

  beforeEach(() => {
    MockMutationCount = 0;
    mockDraftSequence = 0;
    mockAddProviderInstance.mockReset();
    mockUpdateProviderInstance.mockReset();
    mockInvalidateQueries.mockReset();
    mockMessageError.mockReset();
    mockValidate.mockReset();
    MockGetSavePayload.mockReset();
    mockValidate.mockResolvedValue(true);
    MockGetSavePayload.mockImplementation((instanceName: string) => ({
      payload: { llm_factory: 'OpenAI', instance_name: instanceName },
      instanceName,
      isDraft: true,
      apiKind: 'add',
    }));
  });

  const renderTwoDrafts = async () => {
    render(React.createElement(SettingModelV2));
    fireEvent.click(screen.getByTestId('select-provider'));
    await waitFor(() =>
      expect(screen.getAllByTestId('draft-card')).toHaveLength(1),
    );
    fireEvent.click(screen.getByTestId('add-instance-bottom'));
    expect(screen.getAllByTestId('draft-card')).toHaveLength(2);
  };

  it('removes an acknowledged draft while retaining a later failed draft', async () => {
    mockAddProviderInstance
      .mockResolvedValueOnce({ code: 0 })
      .mockResolvedValueOnce({ code: 1 });

    await renderTwoDrafts();

    fireEvent.click(screen.getByTestId('save-all'));

    await waitFor(() => {
      expect(mockAddProviderInstance).toHaveBeenCalledTimes(2);
      expect(screen.getAllByTestId('draft-card')).toHaveLength(1);
    });
    expect(screen.getByTestId('draft-card')).toHaveAttribute(
      'data-instance-name',
      'draft-2',
    );
  });

  it('validates an empty draft even though it has no save payload', async () => {
    MockGetSavePayload.mockReturnValue(null);
    mockValidate.mockResolvedValue(false);

    render(React.createElement(SettingModelV2));
    fireEvent.click(screen.getByTestId('select-provider'));
    await waitFor(() => expect(screen.getByTestId('draft-card')).toBeVisible());
    fireEvent.click(screen.getByTestId('save-all'));

    await waitFor(() => expect(mockValidate).toHaveBeenCalledWith('draft-1'));
    expect(mockAddProviderInstance).not.toHaveBeenCalled();
  });

  it('does not partially save valid drafts when another draft is empty', async () => {
    MockGetSavePayload.mockImplementation((instanceName: string) =>
      instanceName === 'draft-2'
        ? null
        : {
            payload: { llm_factory: 'OpenAI', instance_name: instanceName },
            instanceName,
            isDraft: true,
            apiKind: 'add',
          },
    );
    mockValidate.mockImplementation(
      async (instanceName: string) => instanceName !== 'draft-2',
    );

    await renderTwoDrafts();
    fireEvent.click(screen.getByTestId('save-all'));

    await waitFor(() => expect(mockValidate).toHaveBeenCalledTimes(2));
    expect(mockAddProviderInstance).not.toHaveBeenCalled();
    expect(screen.getAllByTestId('draft-card')).toHaveLength(2);
  });

  it('retains a draft when an existing instance causes the write to be skipped', async () => {
    mockAddProviderInstance
      .mockResolvedValueOnce({ code: 0 })
      .mockResolvedValueOnce({
        code: 0,
        data: null,
        skippedExisting: true,
      });

    await renderTwoDrafts();
    fireEvent.click(screen.getByTestId('save-all'));

    await waitFor(() => {
      expect(mockAddProviderInstance).toHaveBeenCalledTimes(2);
      expect(screen.getAllByTestId('draft-card')).toHaveLength(1);
    });
    expect(screen.getByTestId('draft-card')).toHaveAttribute(
      'data-instance-name',
      'draft-2',
    );
    expect(mockMessageError).toHaveBeenCalledWith('draft-2: nameRepeatedMsg');
  });

  it('disables card interaction while asynchronous validation is pending', async () => {
    let resolveValidation!: (valid: boolean) => void;
    mockValidate.mockReturnValue(
      new Promise<boolean>((resolve) => {
        resolveValidation = resolve;
      }),
    );

    render(React.createElement(SettingModelV2));
    fireEvent.click(screen.getByTestId('select-provider'));
    await waitFor(() => expect(screen.getByTestId('draft-card')).toBeVisible());

    fireEvent.click(screen.getByTestId('save-all'));
    const fieldset = screen.getByTestId('draft-card').closest('fieldset');
    await waitFor(() => expect(fieldset).toBeDisabled());
    expect(mockAddProviderInstance).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId('select-other-provider'));
    expect(screen.getByTestId('draft-card')).toBeVisible();

    await act(async () => resolveValidation(false));
    await waitFor(() => expect(fieldset).not.toBeDisabled());
    expect(screen.getByTestId('draft-card')).toBeVisible();
  });

  it('does not capture a save payload while a model mutation is pending', async () => {
    MockMutationCount = 1;

    render(React.createElement(SettingModelV2));
    fireEvent.click(screen.getByTestId('select-provider'));
    await waitFor(() => expect(screen.getByTestId('draft-card')).toBeVisible());
    fireEvent.click(screen.getByTestId('save-all'));

    expect(MockGetSavePayload).not.toHaveBeenCalled();
    expect(mockAddProviderInstance).not.toHaveBeenCalled();
  });
});
