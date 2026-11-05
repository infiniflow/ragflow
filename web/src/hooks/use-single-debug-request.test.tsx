import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook } from '@testing-library/react';
import { useState } from 'react';
import { useDebugSingle } from './use-single-debug-request';

const mockDebug = jest.fn();
const mockError = jest.fn();

jest.mock('@/services/agent-service', () => ({
  __esModule: true,
  default: { debugSingle: (...args: unknown[]) => mockDebug(...args) },
}));
jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { error: (...args: unknown[]) => mockError(...args) },
}));
jest.mock('react-router', () => ({ useParams: () => ({ id: 'agent-1' }) }));

/** Gives each mounted hook test its own stable React Query client. */
function wrapper({ children }: { children?: any }) {
  const [client] = useState(() => new QueryClient());
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

/** Lets a test resolve requests out of order to check stale-response handling. */
function pendingResponse() {
  let resolve!: (value: any) => void;
  const promise = new Promise((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

const Request = { component_id: 'Agent:1', params: { question: 'Hello' } };
const Snapshot = {
  calls: [
    {
      sequence: 1,
      model: 'test',
      status: 'captured',
      messages: [{ role: 'user', content: 'Hello' }],
    },
  ],
  omitted_count: 0,
};

beforeEach(() => {
  mockDebug.mockReset();
  mockError.mockReset();
});

it('keeps legacy outputs intact even when they contain envelope-like keys', async () => {
  const output = {
    outputs: 'business value',
    model_inputs: 'another business value',
  };
  mockDebug.mockResolvedValue({ data: { code: 0, message: '', data: output } });
  const { result } = renderHook(() => useDebugSingle('Agent:1'), { wrapper });
  await act(async () => {
    await result.current.debugSingle(Request);
  });
  expect(mockDebug.mock.calls[0][0].data).toEqual({ params: Request.params });
  expect(result.current.data).toEqual(output);
  expect(result.current.result?.modelInputs).toBeUndefined();
});

it('unwraps only an opted-in request and retains errors, outputs and captured attempts', async () => {
  mockDebug.mockResolvedValue({
    data: {
      code: 102,
      message: 'Model failed',
      data: { outputs: { _ERROR: 'Provider failed' }, model_inputs: Snapshot },
    },
  });
  const { result } = renderHook(() => useDebugSingle('Agent:1'), { wrapper });
  await act(async () => {
    await result.current.debugSingle({
      ...Request,
      include_model_inputs: true,
    });
  });
  expect(mockDebug.mock.calls[0][0].data).toEqual({
    params: Request.params,
    include_model_inputs: true,
  });
  expect(result.current.data).toEqual({ _ERROR: 'Provider failed' });
  expect(result.current.result).toMatchObject({
    code: 102,
    message: 'Model failed',
    modelInputs: Snapshot,
  });
  expect(result.current.loading).toBe(false);
});

it('retains capture when outputs are empty and handles rejection before capture starts', async () => {
  mockDebug.mockResolvedValueOnce({
    data: {
      code: 102,
      message: 'Failed after invocation',
      data: { outputs: null, model_inputs: Snapshot },
    },
  });
  mockDebug.mockResolvedValueOnce({
    data: { code: 403, message: 'Update permission required', data: null },
  });
  const { result } = renderHook(() => useDebugSingle('Agent:1'), { wrapper });
  await act(async () => {
    await result.current.debugSingle({
      ...Request,
      include_model_inputs: true,
    });
  });
  expect(result.current.result?.modelInputs).toEqual(Snapshot);
  expect(result.current.data).toBeNull();
  await act(async () => {
    await result.current.debugSingle({
      ...Request,
      include_model_inputs: true,
    });
  });
  expect(result.current.result).toMatchObject({
    code: 403,
    message: 'Update permission required',
    outputs: null,
  });
  expect(result.current.result?.modelInputs).toBeUndefined();
});

it('clears the prior result on a new run and ignores an older completion and finalizer', async () => {
  const first = pendingResponse();
  const second = pendingResponse();
  mockDebug.mockResolvedValueOnce({
    data: { code: 0, message: '', data: { content: 'previous' } },
  });
  mockDebug
    .mockReturnValueOnce(first.promise)
    .mockReturnValueOnce(second.promise);
  const { result } = renderHook(() => useDebugSingle('Agent:1'), { wrapper });
  await act(async () => {
    await result.current.debugSingle(Request);
  });
  expect(result.current.data).toEqual({ content: 'previous' });
  let firstRun!: Promise<void>;
  let secondRun!: Promise<void>;
  await act(async () => {
    firstRun = result.current.debugSingle(Request);
  });
  expect(result.current.data).toBeUndefined();
  await act(async () => {
    secondRun = result.current.debugSingle(Request);
  });
  await act(async () => {
    first.resolve({ data: { code: 102, message: 'stale error', data: {} } });
    await firstRun;
  });
  expect(result.current.result).toBeUndefined();
  expect(result.current.loading).toBe(true);
  expect(mockError).not.toHaveBeenCalled();
  await act(async () => {
    second.resolve({
      data: { code: 0, message: '', data: { content: 'current' } },
    });
    await secondRun;
  });
  expect(result.current.data).toEqual({ content: 'current' });
  expect(result.current.loading).toBe(false);
});

it.each(['node', 'close'])(
  'invalidates an in-flight response on %s change',
  async (change) => {
    const pending = pendingResponse();
    mockDebug.mockReturnValue(pending.promise);
    const { result, rerender } = renderHook(
      ({ node, visible }) => useDebugSingle(node, visible),
      { wrapper, initialProps: { node: 'Agent:1', visible: true } },
    );
    let run!: Promise<void>;
    await act(async () => {
      run = result.current.debugSingle(Request);
    });
    rerender({
      node: change === 'node' ? 'Agent:2' : 'Agent:1',
      visible: change !== 'close',
    });
    expect(result.current.loading).toBe(false);
    await act(async () => {
      pending.resolve({
        data: { code: 0, message: '', data: { content: 'stale' } },
      });
      await run;
    });
    expect(result.current.result).toBeUndefined();
  },
);

it('keeps a transport error visible without retaining a previous snapshot', async () => {
  mockDebug.mockRejectedValue(new Error('Connection closed'));
  const { result } = renderHook(() => useDebugSingle('Agent:1'), { wrapper });
  await act(async () => {
    await result.current.debugSingle({
      ...Request,
      include_model_inputs: true,
    });
  });
  expect(result.current.result).toMatchObject({
    code: -1,
    message: 'Connection closed',
    outputs: null,
  });
  expect(result.current.loading).toBe(false);
});
