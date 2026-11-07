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

const mockRefetchAgentLog = jest.fn();
const mockUseFetchAgentLog = jest.fn();
const mockHandleExport = jest.fn();

jest.mock('@/hooks/use-agent-request', () => ({
  useFetchAgentLog: (...args: unknown[]) => mockUseFetchAgentLog(...args),
  useFetchAgent: () => ({
    loading: false,
    data: {},
    refetch: jest.fn(),
  }),
}));

jest.mock('@/hooks/logic-hooks/navigate-hooks', () => ({
  useNavigatePage: () => ({
    navigateToAgents: jest.fn(),
    navigateToAgent: jest.fn(),
  }),
}));

jest.mock('@/pages/agent/hooks/use-fetch-data', () => ({
  useFetchDataOnMount: () => ({ flowDetail: {} }),
}));

jest.mock('../hooks/use-export-agent-log', () => ({
  useExportAgentLogToCSV: () => ({
    handleExport: mockHandleExport,
    loading: false,
  }),
}));

jest.mock('../agent-log-detail-modal', () => ({
  AgentLogDetailModal: () => null,
}));

// DatePickerWithRange pulls in popover + calendar (day-picker) which jsdom
// cannot satisfy. Stub it to a plain button so the layout still renders.
jest.mock('@/components/ui/range-picker', () => ({
  DatePickerWithRange: ({ selected, onSelect }: any) => (
    <button
      type="button"
      data-testid="date-picker"
      onClick={() =>
        onSelect?.({
          from: selected?.from ?? new Date(),
          to: selected?.to ?? new Date(),
        })
      }
    >
      date-picker
    </button>
  ),
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: { page: number }) =>
      key === 'pagination.page' && options ? `${options.page} per page` : key,
  }),
}));

jest.mock('react-router', () => ({
  useParams: () => ({ id: 'canvas-1' }),
}));

import AgentLogPage from '../agent-log-page';

const originalResizeObserver = globalThis.ResizeObserver;
const originalScrollIntoView = Element.prototype.scrollIntoView;

beforeAll(() => {
  globalThis.ResizeObserver = jest.fn().mockImplementation(() => ({
    observe: jest.fn(),
    unobserve: jest.fn(),
    disconnect: jest.fn(),
  }));
  Element.prototype.scrollIntoView = jest.fn();
});

afterAll(() => {
  globalThis.ResizeObserver = originalResizeObserver;
  Element.prototype.scrollIntoView = originalScrollIntoView;
});

describe('AgentLogPage reset button (#17050)', () => {
  beforeEach(() => {
    mockRefetchAgentLog.mockClear();
    mockHandleExport.mockClear();
    mockUseFetchAgentLog.mockReturnValue({
      data: { sessions: [], total: 0 },
      loading: false,
      refetch: mockRefetchAgentLog,
    });
  });

  it('triggers a refetch when Reset is clicked from the default state', () => {
    render(<AgentLogPage />);

    fireEvent.click(screen.getByRole('button', { name: 'common.reset' }));

    expect(mockRefetchAgentLog).toHaveBeenCalledTimes(1);
  });

  it('resets the keyword input back to empty when Reset is clicked', () => {
    render(<AgentLogPage />);

    const searchInput = screen.getByPlaceholderText(
      'common.search',
    ) as HTMLInputElement;
    fireEvent.change(searchInput, { target: { value: 'hello' } });
    expect(searchInput.value).toBe('hello');

    fireEvent.click(screen.getByRole('button', { name: 'common.reset' }));

    expect(searchInput.value).toBe('');
    expect(mockUseFetchAgentLog).toHaveBeenLastCalledWith(
      expect.objectContaining({ keywords: '' }),
    );
    expect(mockRefetchAgentLog).not.toHaveBeenCalled();
  });

  it('resets sorting along with the query and table indicator', () => {
    render(<AgentLogPage />);
    fireEvent.click(screen.getByText('flow.latestDate', { selector: 'th *' }));
    expect(mockUseFetchAgentLog).toHaveBeenLastCalledWith(
      expect.objectContaining({ orderby: 'update_date' }),
    );

    fireEvent.click(screen.getByRole('button', { name: 'common.reset' }));

    expect(mockUseFetchAgentLog).toHaveBeenLastCalledWith(
      expect.objectContaining({ orderby: 'create_time', desc: false }),
    );
    expect(mockRefetchAgentLog).not.toHaveBeenCalled();
  });

  it('resets from page 2 to page 1 while preserving page size and avoiding a stale refetch', () => {
    mockUseFetchAgentLog.mockReturnValue({
      data: { sessions: [], total: 41 },
      loading: false,
      refetch: mockRefetchAgentLog,
    });
    render(<AgentLogPage />);

    fireEvent.click(screen.getByRole('combobox'));
    fireEvent.click(screen.getByRole('option', { name: /^20\b/ }));
    fireEvent.click(screen.getByRole('link', { name: '2' }));
    expect(mockUseFetchAgentLog).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 2, page_size: 20 }),
    );

    fireEvent.click(screen.getByRole('button', { name: 'common.reset' }));

    expect(mockUseFetchAgentLog).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 1, page_size: 20 }),
    );
    expect(mockRefetchAgentLog).not.toHaveBeenCalled();
  });
});
