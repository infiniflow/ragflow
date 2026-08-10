/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 */

const mockHandleVerify = jest.fn();
const mockHandleAddModel = jest.fn();
const mockHandleEditSubmit = jest.fn();
const mockUpdateCatalogModel = jest.fn();
let mockCanBatchToggle = false;
let mockSubmittedModelTypes = ['chat'];
let mockModels = [
  {
    name: 'future-model',
    model_types: [] as string[],
    max_tokens: 8192,
    features: [] as string[],
  },
];

jest.mock('@/components/ui/button', () => {
  const React = jest.requireActual('react');
  return {
    Button: ({ children, variant: _variant, size: _size, ...props }: any) =>
      React.createElement('button', props, children),
  };
});
jest.mock('@/components/ui/checkbox', () => {
  const React = jest.requireActual('react');
  return {
    Checkbox: ({ onCheckedChange, checked, ...props }: any) =>
      React.createElement('button', {
        ...props,
        type: 'button',
        role: 'checkbox',
        'aria-checked': checked === true,
        onClick: () => onCheckedChange?.(checked !== true),
      }),
  };
});
jest.mock('@/components/ui/input', () => {
  const React = jest.requireActual('react');
  return {
    SearchInput: ({ rootClassName: _rootClassName, ...props }: any) =>
      React.createElement('input', props),
  };
});
jest.mock('@/hooks/common-hooks', () => ({
  useCommonTranslation: () => ({ t: (key: string) => key }),
  useTranslate: () => ({ t: (key: string) => key }),
}));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/hooks/use-llm-request', () => ({
  useFetchInstanceModels: () => ({ data: [] }),
}));
jest.mock('../available-models', () => ({ mapModelKey: {} }));
jest.mock('../add-custom-model-dialog', () => ({
  AddCustomModelDialog: ({
    open,
    fields,
    onSubmit,
    onOpenChange,
    loading,
  }: any) => {
    if (!open) return null;
    const typeField = fields.find((field: any) => field.name === 'model_types');
    return (
      <div data-testid="model-dialog">
        <span data-testid="model-types-required">
          {String(Boolean(typeField?.required))}
        </span>
        <button
          type="button"
          data-testid="confirm-model"
          disabled={loading}
          onClick={() =>
            onSubmit({
              name: 'future-model',
              model_types: mockSubmittedModelTypes,
              max_tokens: 8192,
              features: [],
            })
          }
        >
          confirm
        </button>
        <button
          type="button"
          data-testid="cancel-model"
          disabled={loading}
          onClick={() => onOpenChange(false)}
        >
          cancel
        </button>
      </div>
    );
  },
}));
jest.mock('./components/model-row', () => ({
  ModelRow: ({ model, onAdd, onEdit, onVerify, onToggleSelect }: any) => (
    <div data-testid={`model-row-${model.name}`}>
      <button type="button" data-testid="add-model" onClick={onAdd}>
        add
      </button>
      {onEdit && (
        <button type="button" data-testid="edit-model" onClick={onEdit}>
          edit
        </button>
      )}
      {onVerify && (
        <button type="button" data-testid="verify-model" onClick={onVerify}>
          verify
        </button>
      )}
      {onToggleSelect && (
        <button
          type="button"
          data-testid="select-model"
          onClick={onToggleSelect}
        >
          select
        </button>
      )}
    </div>
  ),
}));
jest.mock('./components/tag-filter-button', () => ({
  TagFilterButton: () => null,
}));
jest.mock('./hooks', () => {
  const React = jest.requireActual('react');
  const hasKnownModelTypes = (model: any) =>
    Array.isArray(model.model_types) && model.model_types.length > 0;
  return {
    DRAFT_INSTANCE_SENTINEL: '__draft__',
    hasKnownModelTypes,
    normalizeModelTypes: (value: unknown) =>
      Array.isArray(value) ? value : value ? [value] : [],
    useResolveCreds: () => ({
      resolveCreds: () => ({
        apiKey: 'token',
        baseUrl: '',
        region: 'ap-northeast-1',
        extensions: {},
      }),
    }),
    useModelsCatalog: () => ({
      catalog: mockModels,
      setCatalog: jest.fn(),
      updateCatalogModel: mockUpdateCatalogModel,
      clearCatalogOverride: jest.fn(),
      manualListLoading: false,
      hasFetched: true,
      handleListModels: jest.fn(),
    }),
    useModelsDerived: () => ({
      instanceItems: [],
      models: mockModels,
      addedSet: new Set<string>(),
    }),
    useModelsFilter: () => ({
      search: '',
      tag: null,
      setSearch: jest.fn(),
      setTag: jest.fn(),
      filteredModels: mockModels,
      allTags: [],
    }),
    useModelVerify: () => ({
      verify: {},
      handleVerify: mockHandleVerify,
      batchVerifying: false,
      handleBatchVerify: jest.fn(),
    }),
    useModelMutations: () => ({
      allFilteredAdded: false,
      canBatchToggle: mockCanBatchToggle,
      handleAddModel: mockHandleAddModel,
      handleRemoveModel: jest.fn(),
      handleAddCustom: jest.fn(),
      handleBatchToggleModels: jest.fn(),
      batchLoading: false,
    }),
    useModelEdit: () => {
      const [editingModel, setEditingModel] = React.useState(null);
      return {
        editingModel,
        setEditingModel,
        editModelDialogFields: [
          { name: 'name', type: 'text', disabled: true },
          { name: 'model_types', type: 'multi-select', required: true },
        ],
        editDefaultValues: editingModel ?? undefined,
        handleEditSubmit: mockHandleEditSubmit,
        editLoading: false,
        customModelDialogFields: [],
        providerFeatureKeys: [],
      };
    },
  };
});

import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import React from 'react';
import { ModelsSection } from './index';

const renderSection = () =>
  render(
    React.createElement(ModelsSection, {
      providerName: 'Bedrock',
      instanceName: 'bedrock-tokyo',
      instance: {
        api_key: 'token',
        id: 'instance-id',
        instance_name: 'bedrock-tokyo',
        provider_id: 'provider-id',
        region: 'ap-northeast-1',
        status: 'active',
      },
    }),
  );

describe('ModelsSection availability-only candidates', () => {
  beforeAll(() => {
    (globalThis as any).React = React;
  });

  afterAll(() => {
    delete (globalThis as any).React;
  });

  beforeEach(() => {
    mockCanBatchToggle = false;
    mockSubmittedModelTypes = ['chat'];
    mockModels = [
      {
        name: 'future-model',
        model_types: [],
        max_tokens: 8192,
        features: [],
      },
    ];
    mockHandleVerify.mockReset();
    mockHandleVerify.mockResolvedValue(true);
    mockHandleAddModel.mockReset();
    mockHandleAddModel.mockResolvedValue(true);
    mockHandleEditSubmit.mockReset();
    mockUpdateCatalogModel.mockReset();
  });

  it('requires a type and verifies the candidate before adding it once', async () => {
    renderSection();

    expect(screen.queryByTestId('verify-model')).not.toBeInTheDocument();
    expect(screen.queryByTestId('select-model')).not.toBeInTheDocument();
    expect(screen.getByTestId('models-batch-toggle')).toBeDisabled();

    fireEvent.click(screen.getByTestId('add-model'));
    expect(screen.getByTestId('model-types-required')).toHaveTextContent(
      'true',
    );
    fireEvent.click(screen.getByTestId('confirm-model'));

    await waitFor(() => expect(mockHandleVerify).toHaveBeenCalledTimes(1));
    expect(mockUpdateCatalogModel).not.toHaveBeenCalled();
    expect(mockHandleAddModel).toHaveBeenCalledTimes(1);
    await waitFor(() =>
      expect(screen.queryByTestId('model-dialog')).not.toBeInTheDocument(),
    );
  });

  it('keeps an unverified candidate unattached', async () => {
    mockHandleVerify.mockResolvedValue(false);
    renderSection();

    fireEvent.click(screen.getByTestId('add-model'));
    fireEvent.click(screen.getByTestId('confirm-model'));

    await waitFor(() => expect(mockHandleVerify).toHaveBeenCalledTimes(1));
    expect(mockUpdateCatalogModel).not.toHaveBeenCalled();
    expect(mockHandleAddModel).not.toHaveBeenCalled();
    expect(screen.getByTestId('model-dialog')).toBeInTheDocument();
  });

  it('requires every selected capability to pass singleton verification', async () => {
    mockSubmittedModelTypes = ['chat', 'embedding'];
    mockHandleVerify.mockResolvedValueOnce(true).mockResolvedValueOnce(false);
    renderSection();

    fireEvent.click(screen.getByTestId('add-model'));
    fireEvent.click(screen.getByTestId('confirm-model'));

    await waitFor(() => expect(mockHandleVerify).toHaveBeenCalledTimes(2));
    expect(mockHandleVerify).toHaveBeenNthCalledWith(
      1,
      expect.objectContaining({ model_types: ['chat'] }),
    );
    expect(mockHandleVerify).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({ model_types: ['embedding'] }),
    );
    expect(mockHandleAddModel).not.toHaveBeenCalled();
    expect(screen.getByTestId('model-dialog')).toBeInTheDocument();
  });

  it('locks the dialog while verification and add are pending', async () => {
    let resolveVerify!: (value: boolean) => void;
    mockHandleVerify.mockReturnValue(
      new Promise<boolean>((resolve) => {
        resolveVerify = resolve;
      }),
    );
    renderSection();

    fireEvent.click(screen.getByTestId('add-model'));
    fireEvent.click(screen.getByTestId('confirm-model'));

    await waitFor(() =>
      expect(screen.getByTestId('confirm-model')).toBeDisabled(),
    );
    fireEvent.click(screen.getByTestId('confirm-model'));
    fireEvent.click(screen.getByTestId('cancel-model'));
    expect(mockHandleVerify).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId('model-dialog')).toBeInTheDocument();

    resolveVerify(true);
    await waitFor(() => expect(mockHandleAddModel).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(screen.queryByTestId('model-dialog')).not.toBeInTheDocument(),
    );
  });

  it('keeps the candidate dialog open when persistence is not acknowledged', async () => {
    mockHandleAddModel.mockResolvedValue(false);
    renderSection();

    fireEvent.click(screen.getByTestId('add-model'));
    fireEvent.click(screen.getByTestId('confirm-model'));

    await waitFor(() => expect(mockHandleAddModel).toHaveBeenCalledTimes(1));
    expect(screen.getByTestId('model-dialog')).toBeInTheDocument();
  });

  it('cancels candidate configuration without adding it', () => {
    renderSection();

    fireEvent.click(screen.getByTestId('add-model'));
    fireEvent.click(screen.getByTestId('cancel-model'));

    expect(mockHandleVerify).not.toHaveBeenCalled();
    expect(mockHandleAddModel).not.toHaveBeenCalled();
    expect(screen.queryByTestId('model-dialog')).not.toBeInTheDocument();
  });
});
