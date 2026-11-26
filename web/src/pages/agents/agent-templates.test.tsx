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

import { TooltipProvider } from '@/components/ui/tooltip';
import '@/locales/config';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import AgentTemplates from './agent-templates';

const mockCreateAgent = jest.fn();
const mockNavigateToAgent = jest.fn(() => jest.fn());
const mockNavigateToAgents = jest.fn();

jest.mock('@/hooks/use-agent-request', () => ({
  useFetchAgentTemplates: jest.fn(),
  useCreateAgent: () => ({ loading: false, createAgent: mockCreateAgent }),
}));

jest.mock('@/hooks/logic-hooks/navigate-hooks', () => ({
  useNavigatePage: () => ({
    navigateToAgent: mockNavigateToAgent,
    navigateToAgents: mockNavigateToAgents,
  }),
}));

// The dialog path is covered by create-agent-dialog tests; stub it so these
// tests stay focused on template scoping and its opening interaction.
jest.mock('./create-agent-dialog', () => ({
  CreateAgentDialog: ({ template }: { template?: { id: string } }) => (
    <div data-testid="agent-create-modal">{template?.id}</div>
  ),
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
    update_time: 1760000000000,
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

function renderTemplates() {
  return render(
    <MemoryRouter initialEntries={['/agent-templates']}>
      <TooltipProvider>
        <QueryClientProvider client={new QueryClient()}>
          <AgentTemplates />
        </QueryClientProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  jest.clearAllMocks();
  agentRequest.useFetchAgentTemplates.mockReturnValue(templates);
});

describe('AgentTemplates', () => {
  it('defaults to the Recommended theme', () => {
    renderTemplates();

    expect(screen.getByText('agent-recommended')).toBeInTheDocument();
    // A template without canvas_category is treated as a workflow template.
    expect(screen.queryByText('pipeline-general')).toBeNull();
  });

  it('does not render template update times', () => {
    renderTemplates();

    expect(screen.queryByText(/\d{4}-\d{2}-\d{2}/)).toBeNull();
  });

  it('treats a missing canvas_category as a workflow template', () => {
    renderTemplates();

    // agent-uncategorized is a Marketing theme without canvas_category; it is
    // scoped to Workflow but hidden until the Marketing theme is selected.
    fireEvent.click(screen.getByRole('button', { name: 'Marketing' }));
    expect(screen.getByText('agent-uncategorized')).toBeInTheDocument();
    expect(screen.queryByText('pipeline-general')).toBeNull();
  });

  it('keeps pipeline templates out of the workflow themes', () => {
    renderTemplates();

    fireEvent.click(screen.getByRole('button', { name: 'Other' }));
    expect(screen.queryByText('pipeline-general')).toBeNull();
  });

  it('renders the theme sidebar and current theme heading', () => {
    renderTemplates();
    expect(
      screen.getByRole('button', { name: 'Recommended' }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Marketing' }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('heading', { name: 'Recommended' }),
    ).toBeInTheDocument();
  });

  it('searches current-theme titles and keeps the keyword when changing themes', async () => {
    renderTemplates();
    const searchInput = screen.getByRole('searchbox');

    fireEvent.change(searchInput, {
      target: { value: '  AGENT-MARKETING  ' },
    });

    await waitFor(() => {
      expect(screen.queryByText('agent-recommended')).toBeNull();
    });
    expect(screen.queryByText('agent-marketing')).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: 'Marketing' }));

    expect(await screen.findByText('agent-marketing')).toBeInTheDocument();
    expect(screen.queryByText('agent-uncategorized')).toBeNull();
  });

  it('does not search template descriptions', async () => {
    renderTemplates();

    fireEvent.change(screen.getByRole('searchbox'), {
      target: { value: 'description' },
    });

    await waitFor(() => {
      expect(screen.queryByTestId('template-card')).toBeNull();
    });
  });

  it('opens the existing creation dialog from the Use button', () => {
    renderTemplates();

    fireEvent.click(screen.getByRole('button', { name: 'Use' }));

    expect(screen.getByTestId('agent-create-modal')).toHaveTextContent(
      'agent-recommended',
    );
  });

  it('closes back to the agents list', () => {
    renderTemplates();

    fireEvent.click(screen.getByRole('button', { name: 'Close' }));

    expect(mockNavigateToAgents).toHaveBeenCalledWith();
  });
});
