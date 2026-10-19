import message from '@/components/ui/message';
import { act, renderHook } from '@testing-library/react';
import { useRunDataflow } from './use-run-dataflow';

const mockSend = jest.fn();
const mockSaveGraph = jest.fn();

jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { error: jest.fn() },
}));

jest.mock('@/hooks/use-send-message', () => ({
  useSendMessageBySSE: () => ({ send: mockSend }),
}));

jest.mock('@/locales/config', () => ({
  __esModule: true,
  default: { t: (key: string) => key },
}));

jest.mock('react-router', () => ({
  useParams: () => ({ id: 'agent-1' }),
}));

jest.mock('./use-save-graph', () => ({
  useSaveGraph: () => ({ saveGraph: mockSaveGraph, loading: false }),
}));

const SuccessResponse = {
  response: { status: 200 },
  data: { code: 0, data: { message_id: 'current-run' } },
};

const complete = (result: unknown, isCurrent = () => true) => ({
  result,
  isCurrent,
});

describe('useRunDataflow completion ownership', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockSend.mockReset();
    mockSaveGraph.mockResolvedValue({ code: 0 });
  });

  it.each([
    [
      'success',
      {
        response: { status: 200 },
        data: { code: 0, data: { message_id: 'old-run' } },
      },
    ],
    [
      'HTTP error',
      {
        response: { status: 503 },
        data: {
          code: 503,
          message: 'Old failure',
          data: { message_id: 'old-run' },
        },
      },
    ],
    ['network failure', undefined],
  ])(
    'ignores a superseded %s without replacing the current run or reporting its error',
    async (_name, oldResult) => {
      let finishFirst!: (value: ReturnType<typeof complete>) => void;
      const firstCompletion = new Promise<ReturnType<typeof complete>>(
        (resolve) => {
          finishFirst = resolve;
        },
      );
      mockSend
        .mockReturnValueOnce(firstCompletion)
        .mockResolvedValueOnce(complete(SuccessResponse));
      const setMessageId = jest.fn();
      const { result } = renderHook(() =>
        useRunDataflow({ showLogSheet: jest.fn(), setMessageId }),
      );
      let firstRun!: Promise<unknown>;
      await act(async () => {
        firstRun = result.current.run({ file: [{ id: 'old-file' }] });
      });
      await act(async () => {
        await result.current.run({ file: [{ id: 'current-file' }] });
      });
      setMessageId.mockClear();
      await act(async () => {
        finishFirst(complete(oldResult, () => false));
        await firstRun;
      });
      expect(result.current.uploadedFileData).toEqual({ id: 'current-file' });
      expect(setMessageId).not.toHaveBeenCalled();
      expect(message.error).not.toHaveBeenCalled();
    },
  );

  it('keeps the current successful run and its polling key', async () => {
    mockSend.mockResolvedValue(complete(SuccessResponse));
    const setMessageId = jest.fn();
    const { result } = renderHook(() =>
      useRunDataflow({ showLogSheet: jest.fn(), setMessageId }),
    );
    await act(async () => {
      expect(await result.current.run({ file: [{ id: 'current-file' }] })).toBe(
        'current-run',
      );
    });
    expect(result.current.uploadedFileData).toEqual({ id: 'current-file' });
    expect(setMessageId).toHaveBeenLastCalledWith('current-run');
  });

  it('keeps current HTTP failure reporting and its diagnostic polling key', async () => {
    mockSend.mockResolvedValue(
      complete({
        response: { status: 503 },
        data: {
          code: 503,
          message: 'Current failure',
          data: { message_id: 'failed-run' },
        },
      }),
    );
    const setMessageId = jest.fn();
    const { result } = renderHook(() =>
      useRunDataflow({ showLogSheet: jest.fn(), setMessageId }),
    );
    await act(async () => {
      await result.current.run({ file: [{ id: 'current-file' }] });
    });
    expect(setMessageId).toHaveBeenLastCalledWith('failed-run');
    expect(message.error).toHaveBeenCalledWith('Current failure');
    expect(result.current.uploadedFileData).toBeUndefined();
  });

  it('keeps current network failure reporting', async () => {
    mockSend.mockResolvedValue(complete(undefined));
    const setMessageId = jest.fn();
    const { result } = renderHook(() =>
      useRunDataflow({ showLogSheet: jest.fn(), setMessageId }),
    );
    await act(async () => {
      await result.current.run({ file: [{ id: 'current-file' }] });
    });
    expect(setMessageId).toHaveBeenCalledTimes(1);
    expect(setMessageId).toHaveBeenCalledWith('');
    expect(message.error).toHaveBeenCalledWith('message.requestError');
  });
});
