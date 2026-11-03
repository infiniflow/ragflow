import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import SingleDebugSheet from '.';

const mockDebug = jest.fn();
const mockCopy = jest.fn();
let mockNode: any;
let mockReadOnly = false;

jest.mock('@/services/agent-service', () => ({
  __esModule: true,
  default: { debugSingle: (...args: unknown[]) => mockDebug(...args) },
}));
jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { error: jest.fn() },
}));
jest.mock('react-router', () => ({ useParams: () => ({ id: 'agent-1' }) }));
jest.mock('@/hooks/use-agent-request', () => ({
  useFetchInputForm: () => ({}),
}));
jest.mock('../../context', () => ({ useCanvasReadonly: () => mockReadOnly }));
jest.mock('../../store', () => ({
  __esModule: true,
  default: (selector: any) => selector({ getNode: () => mockNode }),
}));
jest.mock('../../debug-content', () => ({
  __esModule: true,
  default: ({ ok, loading }: any) => (
    <button disabled={loading} onClick={() => ok([])}>
      Run
    </button>
  ),
}));
jest.mock('@/components/ui/sheet', () => ({
  Sheet: ({ open, children }: any) => (open ? <div>{children}</div> : null),
  SheetContent: ({ children }: any) => <div>{children}</div>,
  SheetHeader: ({ children }: any) => <div>{children}</div>,
}));
jest.mock('react18-json-view', () => ({
  __esModule: true,
  default: ({ src }: any) => (
    <pre data-testid="json">{JSON.stringify(src)}</pre>
  ),
}));
jest.mock('@/components/copy-to-clipboard', () => ({
  __esModule: true,
  default: ({ text }: any) => (
    <button onClick={() => mockCopy(text)}>Copy</button>
  ),
}));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: any) =>
      options ? `${key} ${Object.values(options).join(' ')}` : key,
  }),
}));

const Snapshot = {
  calls: [
    {
      sequence: 1,
      model: 'model-a',
      status: 'captured',
      messages: [
        {
          role: 'system',
          content: 'Resolved prompt <script>literal</script>\nnext line',
        },
        { role: 'user', content: 'Question' },
      ],
    },
    {
      sequence: 2,
      model: 'model-a',
      status: 'omitted',
      reason: 'unsupported_content',
    },
  ],
  omitted_count: 1,
};

/** Renders an open Agent test sheet with an isolated React Query cache. */
function renderSheet() {
  const client = new QueryClient();
  return render(
    <QueryClientProvider client={client}>
      <SingleDebugSheet componentId="Agent:1" visible hideModal={jest.fn()} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  mockDebug.mockReset();
  mockCopy.mockReset();
  mockReadOnly = false;
  mockNode = { data: { label: 'Agent', form: {} } };
});

it('offers capture only on editable Agent nodes, defaulting to off', async () => {
  mockDebug.mockResolvedValue({
    data: { code: 0, message: '', data: { content: 'Ordinary output' } },
  });
  renderSheet();
  expect(
    screen.getByRole('switch', { name: 'flow.captureModelInputs' }),
  ).toHaveAttribute('aria-checked', 'false');
  fireEvent.click(screen.getByRole('button', { name: 'Run' }));
  await screen.findByText(/Ordinary output/);
  expect(mockDebug.mock.calls[0][0].data).toEqual({ params: {} });
  expect(
    screen.queryByRole('button', { name: 'flow.modelInputs' }),
  ).not.toBeInTheDocument();
});

it.each(['CodeExec', 'Retrieval', 'Invoke'])(
  'keeps capture out of %s test runs',
  (label) => {
    mockNode = { data: { label, form: {} } };
    renderSheet();
    expect(screen.queryByRole('switch')).not.toBeInTheDocument();
  },
);

it('keeps capture out of read-only previews', () => {
  mockReadOnly = true;
  renderSheet();
  expect(screen.queryByRole('switch')).not.toBeInTheDocument();
});

it('preserves an error and empty outputs with a collapsed, copyable snapshot', async () => {
  mockDebug.mockResolvedValue({
    data: {
      code: 102,
      message: 'Tool failed',
      data: { outputs: {}, model_inputs: Snapshot },
    },
  });
  const { container } = renderSheet();
  fireEvent.click(
    screen.getByRole('switch', { name: 'flow.captureModelInputs' }),
  );
  fireEvent.click(screen.getByRole('button', { name: 'Run' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('102 Tool failed');
  expect(mockDebug.mock.calls[0][0].data.include_model_inputs).toBe(true);
  const expand = screen.getByRole('button', { name: 'flow.modelInputs' });
  expect(expand).toHaveAttribute('aria-expanded', 'false');
  expect(screen.queryByText(/Resolved prompt/)).not.toBeInTheDocument();
  fireEvent.click(expand);
  const json = screen.getByTestId('json');
  expect(json).toHaveTextContent(JSON.stringify(Snapshot));
  expect(container.querySelector('script')).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: 'Copy' }));
  expect(JSON.parse(mockCopy.mock.calls[0][0])).toEqual(Snapshot);
  expect(mockDebug).toHaveBeenCalledTimes(1);
});

it('retains the component error alongside the top-level error and snapshot', async () => {
  mockDebug.mockResolvedValue({
    data: {
      code: 102,
      message: 'Execution failed',
      data: {
        outputs: { _ERROR: 'Specific provider error' },
        model_inputs: Snapshot,
      },
    },
  });
  renderSheet();
  fireEvent.click(screen.getByRole('switch'));
  fireEvent.click(screen.getByRole('button', { name: 'Run' }));
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Execution failed',
  );
  expect(screen.getByTestId('json')).toHaveTextContent(
    'Specific provider error',
  );
  expect(
    screen.getByRole('button', { name: 'flow.modelInputs' }),
  ).toBeInTheDocument();
});

it('explains a successful run with no model calls and copies the empty summary', async () => {
  const empty = { calls: [], omitted_count: 0 };
  mockDebug.mockResolvedValue({
    data: {
      code: 0,
      message: '',
      data: { outputs: null, model_inputs: empty },
    },
  });
  renderSheet();
  fireEvent.click(screen.getByRole('switch'));
  fireEvent.click(screen.getByRole('button', { name: 'Run' }));
  fireEvent.click(
    await screen.findByRole('button', { name: 'flow.modelInputs' }),
  );
  expect(screen.getByText('flow.noModelInputs')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Copy' }));
  expect(JSON.parse(mockCopy.mock.calls[0][0])).toEqual(empty);
});

it('retains CodeExec output sections and never interprets its output keys as capture', async () => {
  mockNode = {
    data: {
      label: 'CodeExec',
      form: { outputs: { result: { type: 'string' } } },
    },
  };
  mockDebug.mockResolvedValue({
    data: {
      code: 0,
      message: '',
      data: {
        result: 'value',
        outputs: 'business outputs',
        model_inputs: 'business capture',
      },
    },
  });
  renderSheet();
  fireEvent.click(screen.getByRole('button', { name: 'Run' }));
  await screen.findByText('Raw Component Output');
  expect(screen.getByText('Business Output')).toBeInTheDocument();
  expect(screen.getByText('System Outputs')).toBeInTheDocument();
  expect(
    screen.queryByRole('button', { name: 'flow.modelInputs' }),
  ).not.toBeInTheDocument();
});

it('clears the snapshot and restores capture to off when another node is selected', async () => {
  mockDebug.mockResolvedValue({
    data: {
      code: 0,
      message: '',
      data: { outputs: {}, model_inputs: Snapshot },
    },
  });
  const client = new QueryClient();
  const view = (componentId: string) => (
    <QueryClientProvider client={client}>
      <SingleDebugSheet
        componentId={componentId}
        visible
        hideModal={jest.fn()}
      />
    </QueryClientProvider>
  );
  const { rerender } = render(view('Agent:1'));
  fireEvent.click(screen.getByRole('switch'));
  fireEvent.click(screen.getByRole('button', { name: 'Run' }));
  await screen.findByRole('button', { name: 'flow.modelInputs' });
  rerender(view('Agent:2'));
  expect(screen.getByRole('switch')).toHaveAttribute('aria-checked', 'false');
  expect(
    screen.queryByRole('button', { name: 'flow.modelInputs' }),
  ).not.toBeInTheDocument();
});
