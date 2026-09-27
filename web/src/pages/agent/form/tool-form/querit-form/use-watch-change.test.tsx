import { act, renderHook, waitFor } from '@testing-library/react';
import { useForm } from 'react-hook-form';
import { useQueritAgentFormChange } from './use-watch-change';

const mockUpdateAgentToolById = jest.fn();
const agentNode = { id: 'agent:0' };

jest.mock('../../../store', () => ({
  __esModule: true,
  default: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      clickedToolId: 'querit:0',
      clickedNodeId: 'agent:0',
      findUpstreamNodeById: () => agentNode,
      updateAgentToolById: mockUpdateAgentToolById,
    }),
}));

const validValues = {
  api_key: '',
  count: 10,
  chunks_per_doc: 3,
  site_include: [],
  site_exclude: [],
  time_range: '',
  country_include: [],
  language_include: [],
};

function useQueritAgentFormWatcher() {
  const form = useForm<Record<string, any>>({ defaultValues: validValues });
  useQueritAgentFormChange(form);
  return form;
}

describe('useQueritAgentFormChange', () => {
  beforeEach(() => {
    mockUpdateAgentToolById.mockClear();
  });

  it('does not write untouched initial values to the Agent node', async () => {
    renderHook(() => useQueritAgentFormWatcher());

    await act(async () => {
      await Promise.resolve();
    });

    expect(mockUpdateAgentToolById).not.toHaveBeenCalled();
  });

  it('writes normalized valid edits without a query', async () => {
    const { result } = renderHook(() => useQueritAgentFormWatcher());

    act(() => {
      result.current.setValue(
        'site_include',
        [{ value: ' docs.example.com ' }],
        { shouldDirty: true },
      );
      result.current.setValue('count', '5', { shouldDirty: true });
    });

    await waitFor(() => {
      expect(mockUpdateAgentToolById).toHaveBeenLastCalledWith(
        agentNode,
        'querit:0',
        {
          params: expect.objectContaining({
            count: 5,
            site_include: ['docs.example.com'],
          }),
        },
      );
    });

    const persistedParams =
      mockUpdateAgentToolById.mock.calls.at(-1)?.[2]?.params;
    expect(persistedParams).not.toHaveProperty('query');
  });

  it('does not overwrite Agent tool params after an invalid edit', async () => {
    const { result } = renderHook(() => useQueritAgentFormWatcher());

    act(() => {
      result.current.setValue('time_range', 'last week', {
        shouldDirty: true,
      });
    });

    await act(async () => {
      await Promise.resolve();
    });

    expect(mockUpdateAgentToolById).not.toHaveBeenCalled();
  });
});
