/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 */

const MockHandleVerify = jest.fn();
const MockHandleAddModel = jest.fn();
const MockHandleEditSubmit = jest.fn();
const MockUpdateCatalogModel = jest.fn();
let MockCanBatchToggle = false;
let MockSubmittedModelTypes = ['chat'];
let MockModels = [
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
    const handleConfirm = () =>
      onSubmit({
        name: 'future-model',
        model_types: MockSubmittedModelTypes,
        max_tokens: 8192,
        features: [],
      });
    const handleCancel = () => onOpenChange(false);
    return (
      <div data-testid="model-dialog">
        <span data-testid="model-types-required">
          {String(Boolean(typeField?.required))}
        </span>
        <button
          type="button"
          data-testid="confirm-model"
          disabled={loading}
          onClick={handleConfirm}
        >
          confirm
        </button>
        <button
          type="button"
          data-testid="cancel-model"
          disabled={loading}
          onClick={handleCancel}
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
      catalog: MockModels,
      setCatalog: jest.fn(),
      updateCatalogModel: MockUpdateCatalogModel,
      clearCatalogOverride: jest.fn(),
      manualListLoading: false,
      hasFetched: true,
      handleListModels: jest.fn(),
    }),
    useModelsDerived: () => ({
      instanceItems: [],
      models: MockModels,
      addedSet: new Set<string>(),
    }),
    useModelsFilter: () => ({
      search: '',
      tag: null,
      setSearch: jest.fn(),
      setTag: jest.fn(),
      filteredModels: MockModels,
      allTags: [],
    }),
    useModelVerify: () => ({
      verify: {},
      handleVerify: MockHandleVerify,
      batchVerifying: false,
      handleBatchVerify: jest.fn(),
    }),
    useModelMutations: () => ({
      allFilteredAdded: false,
      canBatchToggle: MockCanBatchToggle,
      handleAddModel: MockHandleAddModel,
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
        handleEditSubmit: MockHandleEditSubmit,
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

const RenderSection = () =>
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
    MockCanBatchToggle = false;
    MockSubmittedModelTypes = ['chat'];
    MockModels = [
      {
        name: 'future-model',
        model_types: [],
        max_tokens: 8192,
        features: [],
      },
    ];
    MockHandleVerify.mockReset();
    MockHandleVerify.mockResolvedValue(true);
    MockHandleAddModel.mockReset();
    MockHandleAddModel.mockResolvedValue(true);
    MockHandleEditSubmit.mockReset();
    MockUpdateCatalogModel.mockReset();
  });

  it('requires a type and verifies the candidate before adding it once', async () => {
    RenderSection();

    expect(screen.queryByTestId('verify-model')).not.toBeInTheDocument();
    expect(screen.queryByTestId('select-model')).not.toBeInTheDocument();
    expect(screen.getByTestId('models-batch-toggle')).toBeDisabled();

    fireEvent.click(screen.getByTestId('add-model'));
    expect(screen.getByTestId('model-types-required')).toHaveTextContent(
      'true',
    );
    fireEvent.click(screen.getByTestId('confirm-model'));

    await waitFor(() => expect(MockHandleVerify).toHaveBeenCalledTimes(1));
    expect(MockUpdateCatalogModel).not.toHaveBeenCalled();
    expect(MockHandleAddModel).toHaveBeenCalledTimes(1);
    await waitFor(() =>
      expect(screen.queryByTestId('model-dialog')).not.toBeInTheDocument(),
    );
  });

  it('keeps an unverified candidate unattached', async () => {
    MockHandleVerify.mockResolvedValue(false);
    RenderSection();

    fireEvent.click(screen.getByTestId('add-model'));
    fireEvent.click(screen.getByTestId('confirm-model'));

    await waitFor(() => expect(MockHandleVerify).toHaveBeenCalledTimes(1));
    expect(MockUpdateCatalogModel).not.toHaveBeenCalled();
    expect(MockHandleAddModel).not.toHaveBeenCalled();
    expect(screen.getByTestId('model-dialog')).toBeInTheDocument();
  });

  it('requires every selected capability to pass singleton verification', async () => {
    MockSubmittedModelTypes = ['chat', 'embedding'];
    MockHandleVerify.mockResolvedValueOnce(true).mockResolvedValueOnce(false);
    RenderSection();

    fireEvent.click(screen.getByTestId('add-model'));
    fireEvent.click(screen.getByTestId('confirm-model'));

    await waitFor(() => expect(MockHandleVerify).toHaveBeenCalledTimes(2));
    expect(MockHandleVerify).toHaveBeenNthCalledWith(
      1,
      expect.objectContaining({ model_types: ['chat'] }),
    );
    expect(MockHandleVerify).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({ model_types: ['embedding'] }),
    );
    expect(MockHandleAddModel).not.toHaveBeenCalled();
    expect(screen.getByTestId('model-dialog')).toBeInTheDocument();
  });

  it('locks the dialog while verification and add are pending', async () => {
    let resolveVerify!: (value: boolean) => void;
    MockHandleVerify.mockReturnValue(
      new Promise<boolean>((resolve) => {
        resolveVerify = resolve;
      }),
    );
    RenderSection();

    fireEvent.click(screen.getByTestId('add-model'));
    fireEvent.click(screen.getByTestId('confirm-model'));

    await waitFor(() =>
      expect(screen.getByTestId('confirm-model')).toBeDisabled(),
    );
    fireEvent.click(screen.getByTestId('confirm-model'));
    fireEvent.click(screen.getByTestId('cancel-model'));
    expect(MockHandleVerify).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId('model-dialog')).toBeInTheDocument();

    resolveVerify(true);
    await waitFor(() => expect(MockHandleAddModel).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(screen.queryByTestId('model-dialog')).not.toBeInTheDocument(),
    );
  });

  it('keeps the candidate dialog open when persistence is not acknowledged', async () => {
    MockHandleAddModel.mockResolvedValue(false);
    RenderSection();

    fireEvent.click(screen.getByTestId('add-model'));
    fireEvent.click(screen.getByTestId('confirm-model'));

    await waitFor(() => expect(MockHandleAddModel).toHaveBeenCalledTimes(1));
    expect(screen.getByTestId('model-dialog')).toBeInTheDocument();
  });

  it('cancels candidate configuration without adding it', () => {
    RenderSection();

    fireEvent.click(screen.getByTestId('add-model'));
    fireEvent.click(screen.getByTestId('cancel-model'));

    expect(MockHandleVerify).not.toHaveBeenCalled();
    expect(MockHandleAddModel).not.toHaveBeenCalled();
    expect(screen.queryByTestId('model-dialog')).not.toBeInTheDocument();
  });
});
