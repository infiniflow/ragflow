import { act, renderHook, waitFor } from '@testing-library/react';
import { StrictMode } from 'react';
import { useForm } from 'react-hook-form';
import { useWatchFormChange } from './use-watch-change';

const mockUpdateNodeForm = jest.fn();

jest.mock('../../store', () => ({
  __esModule: true,
  default: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({ updateNodeForm: mockUpdateNodeForm }),
}));

const validValues = {
  api_key: '',
  query: 'begin.query',
  count: 10,
  chunks_per_doc: 3,
  site_include: [],
  site_exclude: [],
  time_range: '',
  country_include: [],
  language_include: [],
};

function useQueritFormWatcher(defaultValues: Record<string, unknown>) {
  const form = useForm({ defaultValues });
  useWatchFormChange('querit:0', form);
  return form;
}

describe('useWatchFormChange', () => {
  beforeEach(() => {
    mockUpdateNodeForm.mockClear();
  });

  it('skips the initial write and normalizes subsequent valid edits', async () => {
    const { result } = renderHook(() =>
      useQueritFormWatcher({
        ...validValues,
        count: '5',
        chunks_per_doc: '2',
        site_include: [{ value: ' docs.example.com ' }],
      }),
    );

    expect(mockUpdateNodeForm).not.toHaveBeenCalled();

    act(() => {
      result.current.setValue('count', '6');
    });

    await waitFor(() => {
      expect(mockUpdateNodeForm).toHaveBeenCalledWith(
        'querit:0',
        expect.objectContaining({
          count: 6,
          chunks_per_doc: 2,
          site_include: ['docs.example.com'],
        }),
      );
    });
  });

  it('does not write an invalid initial form to the Canvas node', async () => {
    renderHook(() =>
      useQueritFormWatcher({
        ...validValues,
        count: 0,
      }),
    );

    await act(async () => {
      await Promise.resolve();
    });

    expect(mockUpdateNodeForm).not.toHaveBeenCalled();
  });

  it('does not write a valid initial form in Strict Mode', async () => {
    renderHook(() => useQueritFormWatcher(validValues), {
      wrapper: StrictMode,
    });

    await act(async () => {
      await Promise.resolve();
    });

    expect(mockUpdateNodeForm).not.toHaveBeenCalled();
  });

  it('does not overwrite the last valid state after an invalid edit', async () => {
    const { result } = renderHook(() => useQueritFormWatcher(validValues));

    act(() => {
      result.current.setValue('count', 5);
    });

    await waitFor(() => {
      expect(mockUpdateNodeForm).toHaveBeenCalled();
    });
    mockUpdateNodeForm.mockClear();

    act(() => {
      result.current.setValue('time_range', 'last week');
    });

    await act(async () => {
      await Promise.resolve();
    });

    expect(mockUpdateNodeForm).not.toHaveBeenCalled();
  });
});
