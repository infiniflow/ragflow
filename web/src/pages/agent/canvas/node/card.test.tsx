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

import { render, screen } from '@testing-library/react';
import { ResourceTenantIdContext } from '../../context';
import { ModelResourceAccessUserIdContext } from '@/hooks/use-llm-request';
import { CompilationTemplateLabelCard, LLMLabelCard } from './card';

const mockUseModelValidIds = jest.fn();
const mockUseCompilationTemplateGroupOptions = jest.fn();
const mockUseCompilationTemplateGroupValidIds = jest.fn();

jest.mock('react-i18next', () => ({
  ...jest.requireActual('react-i18next'),
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/hooks/use-llm-request', () => ({
  ModelResourceAccessUserIdContext: jest
    .requireActual('react')
    .createContext(undefined),
  useModelValidIds: (...args: unknown[]) => mockUseModelValidIds(...args),
}));

jest.mock('@/hooks/use-compilation-template-group-request', () => ({
  useCompilationTemplateGroupOptions: (...args: unknown[]) =>
    mockUseCompilationTemplateGroupOptions(...args),
  useCompilationTemplateGroupValidIds: (...args: unknown[]) =>
    mockUseCompilationTemplateGroupValidIds(...args),
}));

jest.mock('@/ee/components/model-reference-label', () => ({
  ModelReferenceLabel: ({ value }: { value?: string }) => (
    <span>{value === 'owner-model' ? 'deepseek-v4-flash' : value}</span>
  ),
}));

function renderCard(llmId?: string) {
  return render(
    <ResourceTenantIdContext.Provider value="owner-tenant">
      <LLMLabelCard llmId={llmId} />
    </ResourceTenantIdContext.Provider>,
  );
}

function renderCurrentUserCard(llmId?: string) {
  return render(
    <ResourceTenantIdContext.Provider value={undefined}>
      <LLMLabelCard llmId={llmId} />
    </ResourceTenantIdContext.Provider>,
  );
}

function renderGroupCard(groupId?: string) {
  return render(
    <ModelResourceAccessUserIdContext.Provider value="owner-user">
      <CompilationTemplateLabelCard groupId={groupId} />
    </ModelResourceAccessUserIdContext.Provider>,
  );
}

describe('LLMLabelCard', () => {
  it('flags a model missing from the current user list, keeping the owner-resolved name', () => {
    mockUseModelValidIds.mockReturnValue({
      validIds: new Set(['m1']),
      isFetched: true,
    });

    const { container } = renderCard('owner-model');

    expect(screen.getByText('deepseek-v4-flash')).toBeInTheDocument();
    expect(screen.getByTitle('common.modelUnavailable')).toBeInTheDocument();
    expect(container.querySelector('.border-state-error')).not.toBeNull();
    expect(container.querySelector('svg.text-state-error')).not.toBeNull();
  });

  it('shows no error state for a usable model', () => {
    mockUseModelValidIds.mockReturnValue({
      validIds: new Set(['owner-model']),
      isFetched: true,
    });

    const { container } = renderCard('owner-model');

    expect(screen.queryByTitle('common.modelUnavailable')).toBeNull();
    expect(container.querySelector('.border-state-error')).toBeNull();
    expect(container.querySelector('svg.text-state-error')).toBeNull();
  });

  it('stays neutral while the model list is loading', () => {
    mockUseModelValidIds.mockReturnValue({
      validIds: new Set(),
      isFetched: false,
    });

    const { container } = renderCard('owner-model');

    expect(screen.queryByTitle('common.modelUnavailable')).toBeNull();
    expect(container.querySelector('.border-state-error')).toBeNull();
  });

  it('keeps the historical red state for an empty model', () => {
    mockUseModelValidIds.mockReturnValue({
      validIds: new Set(['m1']),
      isFetched: true,
    });

    const { container } = renderCard(undefined);

    expect(container.querySelector('.border-state-error')).not.toBeNull();
    expect(screen.queryByTitle('common.modelUnavailable')).toBeNull();
    expect(container.querySelector('svg.text-state-error')).toBeNull();
  });

  it('shows group pending for the current user without an unavailable error state', () => {
    const groupRef = 'gp-0123456789abcdef0123456789abcdef';
    const pendingMessage = 'setting.modelGroupSelector.pending';
    mockUseModelValidIds.mockReturnValue({
      validIds: new Set(),
      isFetched: true,
      getPendingMessage: () => pendingMessage,
    });

    const { container } = renderCurrentUserCard(groupRef);

    expect(mockUseModelValidIds).toHaveBeenCalledWith(
      ['chat', 'vision'],
      undefined,
    );
    expect(screen.getByTitle(pendingMessage)).toBeInTheDocument();
    expect(screen.queryByTitle('common.modelUnavailable')).toBeNull();
    expect(container.querySelector('.border-state-error')).toBeNull();
    expect(container.querySelector('svg.text-state-error')).toBeNull();
  });

  it('flags an unknown settled group reference as unavailable for the current user', () => {
    const groupRef = 'gp-0123456789abcdef0123456789abcdef';
    mockUseModelValidIds.mockReturnValue({
      validIds: new Set(),
      isFetched: true,
      isSettled: true,
      getPendingMessage: () => undefined,
    });

    const { container } = renderCurrentUserCard(groupRef);

    expect(screen.getByTitle('common.modelUnavailable')).toBeInTheDocument();
    expect(container.querySelector('.border-state-error')).not.toBeNull();
    expect(container.querySelector('svg.text-state-error')).not.toBeNull();
  });

  it('validates against the canvas owner tenant', () => {
    mockUseModelValidIds.mockReturnValue({
      validIds: new Set(['m1']),
      isFetched: true,
    });

    renderCard('m1');

    expect(mockUseModelValidIds).toHaveBeenCalledWith(
      ['chat', 'vision'],
      'owner-tenant',
    );
  });

  it('accepts decision models when Categorize includes them', () => {
    mockUseModelValidIds.mockImplementation((types: string[]) => ({
      validIds: new Set(types.includes('decision') ? ['decision-model'] : []),
      isFetched: true,
    }));

    const { container } = render(
      <ResourceTenantIdContext.Provider value="owner-tenant">
        <LLMLabelCard
          llmId="decision-model"
          modelTypes={['chat', 'vision', 'decision']}
        />
      </ResourceTenantIdContext.Provider>,
    );

    expect(mockUseModelValidIds).toHaveBeenCalledWith(
      ['chat', 'vision', 'decision'],
      'owner-tenant',
    );
    expect(screen.queryByTitle('common.modelUnavailable')).toBeNull();
    expect(container.querySelector('.border-state-error')).toBeNull();
  });
});

describe('CompilationTemplateLabelCard', () => {
  beforeEach(() => {
    mockUseCompilationTemplateGroupOptions.mockReturnValue({
      options: [{ label: 'Group A', value: 'g1' }],
      isFetched: true,
      isError: false,
    });
  });

  it('flags a group the current user cannot resolve', () => {
    mockUseCompilationTemplateGroupValidIds.mockReturnValue({
      validIds: new Set(['g1']),
      isFetched: true,
    });

    const { container } = renderGroupCard('gone');

    expect(screen.getByText('gone')).toBeInTheDocument();
    expect(
      screen.getByTitle(
        'knowledgeConfiguration.compilationTemplateUnavailable',
      ),
    ).toBeInTheDocument();
    expect(container.querySelector('.border-state-error')).not.toBeNull();
    expect(container.querySelector('svg.text-state-error')).not.toBeNull();
  });

  it('shows no error state for a resolvable group', () => {
    mockUseCompilationTemplateGroupValidIds.mockReturnValue({
      validIds: new Set(['g1']),
      isFetched: true,
    });

    const { container } = renderGroupCard('g1');

    expect(screen.getByText('Group A')).toBeInTheDocument();
    expect(
      screen.queryByTitle(
        'knowledgeConfiguration.compilationTemplateUnavailable',
      ),
    ).toBeNull();
    expect(container.querySelector('.border-state-error')).toBeNull();
    expect(container.querySelector('svg.text-state-error')).toBeNull();
  });

  it('stays neutral while the group list is loading', () => {
    mockUseCompilationTemplateGroupValidIds.mockReturnValue({
      validIds: new Set(),
      isFetched: false,
    });

    const { container } = renderGroupCard('gone');

    expect(
      screen.queryByTitle(
        'knowledgeConfiguration.compilationTemplateUnavailable',
      ),
    ).toBeNull();
    expect(container.querySelector('.border-state-error')).toBeNull();
  });

  it('stays neutral for an empty group — the checklist covers unconfigured nodes', () => {
    mockUseCompilationTemplateGroupValidIds.mockReturnValue({
      validIds: new Set(['g1']),
      isFetched: true,
    });

    const { container } = renderGroupCard(undefined);

    expect(container.querySelector('.border-state-error')).toBeNull();
    expect(
      screen.queryByTitle(
        'knowledgeConfiguration.compilationTemplateUnavailable',
      ),
    ).toBeNull();
    expect(container.querySelector('svg.text-state-error')).toBeNull();
  });

  it('resolves names and validity against the canvas owner user', () => {
    mockUseCompilationTemplateGroupValidIds.mockReturnValue({
      validIds: new Set(['g1']),
      isFetched: true,
    });

    renderGroupCard('g1');

    expect(mockUseCompilationTemplateGroupOptions).toHaveBeenCalledWith(
      'owner-user',
    );
    expect(mockUseCompilationTemplateGroupValidIds).toHaveBeenCalledWith(
      'owner-user',
    );
  });
});
