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

import { fireEvent, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import AgentTemplates from './agent-templates';

const mockCreateAgent = jest.fn();
const mockNavigateToAgent = jest.fn(() => jest.fn());

jest.mock('@/hooks/use-agent-request', () => ({
  useFetchAgentTemplates: jest.fn(),
  useCreateAgent: () => ({ loading: false, createAgent: mockCreateAgent }),
}));

jest.mock('@/hooks/logic-hooks/navigate-hooks', () => ({
  useNavigatePage: () => ({ navigateToAgent: mockNavigateToAgent }),
}));

// The dialog path is covered by create-agent-dialog tests; stub it so these
// tests stay focused on template scoping.
jest.mock('./create-agent-dialog', () => ({
  CreateAgentDialog: () => null,
}));

// eslint-disable-next-line @typescript-eslint/no-var-requires
const agentRequest = jest.requireMock('@/hooks/use-agent-request');

function makeTemplate(id: string, canvasType: string, canvasCategory?: string) {
  return {
    id,
    avatar: '',
    canvas_type: canvasType,
    canvas_category: canvasCategory,
    create_date: '',
    create_time: 0,
    update_date: '',
    update_time: 0,
    dsl: { graph: { nodes: [], edges: [] }, components: {}, history: [] },
    title: { en: id, zh: id, de: id },
    description: { en: `${id} description`, zh: '', de: '' },
  };
}

const templates = [
  makeTemplate('agent-recommended', 'Recommended'),
  makeTemplate('agent-marketing', 'Marketing'),
  makeTemplate('agent-uncategorized', 'Marketing', undefined),
  makeTemplate('pipeline-general', 'Ingestion Pipeline', 'dataflow_canvas'),
];

function renderTemplates(initialEntry = '/agent-templates') {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <QueryClientProvider client={new QueryClient()}>
        <AgentTemplates />
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  jest.clearAllMocks();
  agentRequest.useFetchAgentTemplates.mockReturnValue(templates);
});

describe('AgentTemplates source scoping', () => {
  it('defaults to the Workflow catalog on the Recommended theme', () => {
    renderTemplates();

    expect(screen.getByText('agent-recommended')).toBeInTheDocument();
    // A template without canvas_category is treated as a workflow template.
    expect(screen.queryByText('pipeline-general')).toBeNull();
  });

  it('shows only pipeline templates without a sidebar on the Pipeline source', () => {
    renderTemplates('/agent-templates?canvas_category=dataflow_canvas');

    expect(screen.getByText('pipeline-general')).toBeInTheDocument();
    expect(screen.queryByText('agent-recommended')).toBeNull();
    expect(screen.queryByText('agent-marketing')).toBeNull();
    // The pipeline catalog has no theme breakdown, so the sidebar is hidden.
    expect(screen.queryByText('Recommended')).toBeNull();
  });

  it('treats a missing canvas_category as a workflow template', () => {
    renderTemplates('/agent-templates');

    // agent-uncategorized is a Marketing theme without canvas_category; it is
    // scoped to Workflow but hidden until the Marketing theme is selected.
    fireEvent.click(screen.getByText('Marketing'));
    expect(screen.getByText('agent-uncategorized')).toBeInTheDocument();
    expect(screen.queryByText('pipeline-general')).toBeNull();
  });

  it('keeps pipeline templates out of the workflow themes', () => {
    renderTemplates('/agent-templates');

    fireEvent.click(screen.getByText('Other'));
    expect(screen.queryByText('pipeline-general')).toBeNull();
  });

  it('renders the theme sidebar for the Workflow source', () => {
    renderTemplates('/agent-templates');
    expect(screen.getByText('Recommended')).toBeInTheDocument();
    expect(screen.getByText('Marketing')).toBeInTheDocument();
  });
});
