import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react';
import { useEffect as mockUseEffect } from 'react';
import { useFormContext as mockUseFormContext } from 'react-hook-form';
import { AxiosHeaders } from 'axios';
import { applyDatasetParserConfig } from '@/services/knowledge-service';
import DatasetSetting from './index';

const mockSave = jest.fn();
const mockTranslate = (key: string) => key;
const mockInitialValues = {
  name: 'Dataset',
  parse_type: 1,
  parser_id: 'general',
  pipeline_id: '',
  pipeline_version_id: '',
  parser_config: { 'Parser:One': { value: 1 } },
};
let mockDataset = { id: 'kb-1', document_count: 2, chunk_count: 0 };
const mockSourceData: unknown[] = [];

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: mockTranslate }),
}));
jest.mock('@/services/knowledge-service', () => ({
  applyDatasetParserConfig: jest.fn(),
}));
jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { success: jest.fn() },
}));
jest.mock('./form-schema', () => {
  const { z } = require('zod');
  return { formSchema: z.object({}).passthrough() };
});
jest.mock('./hooks', () => ({
  useFetchDatasetSettingOnMount: (form: {
    reset: (values: unknown) => void;
  }) => {
    mockUseEffect(() => form.reset(mockInitialValues), [form]);
    return {
      knowledgeDetails: mockDataset,
      loading: false,
      sourceData: mockSourceData,
    };
  },
  useSaveDatasetSetting: () => ({ handleSave: mockSave, loading: false }),
  usePipelineDataList: () => [],
  useConnectorHandlers: () => ({}),
}));
jest.mock('./general-form', () => ({
  GeneralForm: () => {
    const form = mockUseFormContext();
    return (
      <>
        <input aria-label="name" {...form.register('name')} />
        <input
          aria-label="parser value"
          {...form.register('parser_config.Parser:One.value', {
            valueAsNumber: true,
          })}
        />
        <input
          aria-label="pipeline version"
          {...form.register('pipeline_version_id')}
        />
      </>
    );
  },
}));
jest.mock('@/components/parser-select', () => ({ ParserSelect: () => null }));
jest.mock('@/components/pipeline-version-select', () => ({
  PipelineVersionSelect: () => null,
}));
jest.mock('@/components/pipeline-operator-tabs', () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock('./chunk-method-learn-more', () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock('./components/link-data-source', () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock('@/hooks/use-parser-select-handler', () => ({
  useParserSelectHandler: () => jest.fn(),
}));
jest.mock('@/hooks/use-parser-options', () => ({
  buildParserSelectValue: () => '',
}));
jest.mock('@/hooks/use-pipeline-operator', () => ({
  useActiveTab: () => ({ activeTab: '', setActiveTab: jest.fn() }),
  usePipelineOperatorNodes: () => ({ operatorNodes: [], loading: false }),
  useResetParserConfigOnPipelineChange: jest.fn(),
}));
jest.mock('@/utils/pipeline-operator', () => ({
  buildParserConfigFromNodes: () => ({}),
}));
jest.mock('./parser-config', () => ({
  normalizeParserConfigForComparison: (config: unknown) => config,
}));
jest.mock('@/hooks/use-agent-request', () => ({
  useFetchCanvasVersionList: () => ({ data: [] }),
}));
jest.mock('./use-reveal-submit-errors', () => ({
  useRevealSubmitErrors: () => ({}),
}));

const mockApply = jest.mocked(applyDatasetParserConfig);

function response(data: Record<string, unknown>) {
  return {
    data,
    status: 200,
    statusText: 'OK',
    headers: {},
    config: { headers: new AxiosHeaders() },
  };
}

function setup() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={queryClient}>
      <DatasetSetting />
    </QueryClientProvider>,
  );
}

function saveParserChanges() {
  fireEvent.change(screen.getByLabelText('parser value'), {
    target: { value: '2' },
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeConfiguration.save' }),
  );
}

beforeEach(() => {
  mockSave.mockReset().mockResolvedValue({ code: 0 });
  mockApply.mockReset().mockResolvedValue(
    response({
      code: 0,
      data: {
        dataset_id: 'kb-1',
        total_documents: 2,
        updated_documents: 2,
        unchanged_documents: 0,
      },
    }),
  );
  mockDataset = { id: 'kb-1', document_count: 2, chunk_count: 0 };
});

it('saves before prompting, including existing files without chunks, and defaults to No', async () => {
  let finishSave!: (value: { code: number }) => void;
  mockSave.mockReturnValue(
    new Promise((resolve) => {
      finishSave = resolve;
    }),
  );
  setup();
  saveParserChanges();
  await waitFor(() => expect(mockSave).toHaveBeenCalledTimes(1));
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
  expect(mockApply).not.toHaveBeenCalled();

  await act(async () => finishSave({ code: 0 }));
  await screen.findByRole('alertdialog');
  expect(
    Array.from(screen.getByRole('alertdialog').querySelectorAll('button')),
  ).toEqual([
    screen.getByRole('button', { name: 'common.yes' }),
    screen.getByRole('button', { name: 'common.no' }),
  ]);
  expect(screen.getByRole('button', { name: 'common.no' })).toHaveFocus();
  expect(screen.getByRole('button', { name: 'common.no' })).toHaveClass(
    'bg-text-primary',
    'text-bg-base',
  );
  expect(screen.getByRole('button', { name: 'common.yes' })).toHaveClass(
    'bg-bg-input',
    'text-text-secondary',
  );
  fireEvent.click(screen.getByRole('button', { name: 'common.no' }));
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument(),
  );
  expect(mockApply).not.toHaveBeenCalled();
  expect(mockSave).toHaveBeenCalledTimes(1);
});

it('applies the saved settings only after Yes', async () => {
  setup();
  saveParserChanges();
  await screen.findByRole('alertdialog');
  expect(mockApply).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'common.yes' }));
  await waitFor(() => expect(mockApply).toHaveBeenCalledWith('kb-1'));
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument(),
  );
  expect(mockSave).toHaveBeenCalledTimes(1);
});

it('does not prompt when saving fails', async () => {
  mockSave.mockResolvedValue({ code: 101 });
  setup();
  saveParserChanges();
  await waitFor(() => expect(mockSave).toHaveBeenCalledTimes(1));
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
  expect(mockApply).not.toHaveBeenCalled();
});

it.each(['unchanged parser settings', 'empty dataset'])(
  'does not prompt for %s',
  async (scenario) => {
    if (scenario === 'empty dataset') mockDataset.document_count = 0;
    setup();
    if (scenario === 'unchanged parser settings') {
      fireEvent.change(screen.getByLabelText('name'), {
        target: { value: 'Renamed' },
      });
      fireEvent.click(
        screen.getByRole('button', { name: 'knowledgeConfiguration.save' }),
      );
    } else {
      saveParserChanges();
    }
    await waitFor(() => expect(mockSave).toHaveBeenCalledTimes(1));
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
  },
);

it('prompts for a pipeline version change', async () => {
  setup();
  fireEvent.change(screen.getByLabelText('pipeline version'), {
    target: { value: 'version-2' },
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeConfiguration.save' }),
  );
  await screen.findByRole('alertdialog');
  expect(mockApply).not.toHaveBeenCalled();
});

it('keeps the dialog open after failure and retries without saving again', async () => {
  mockApply.mockResolvedValueOnce(
    response({ code: 409, message: 'Stop parsing first' }),
  );
  setup();
  saveParserChanges();
  await screen.findByRole('alertdialog');
  fireEvent.click(screen.getByRole('button', { name: 'common.yes' }));
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'knowledgeConfiguration.parserSettingsApplyFailed',
  );
  expect(screen.getByRole('alert')).toHaveTextContent('Stop parsing first');
  fireEvent.click(screen.getByRole('button', { name: 'common.yes' }));
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument(),
  );
  expect(mockApply).toHaveBeenCalledTimes(2);
  expect(mockSave).toHaveBeenCalledTimes(1);
});

it('treats Escape as No', async () => {
  setup();
  saveParserChanges();
  const dialog = await screen.findByRole('alertdialog');
  fireEvent.keyDown(dialog, { key: 'Escape', code: 'Escape' });
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument(),
  );
  expect(mockApply).not.toHaveBeenCalled();
});

it('disables both choices while applying settings', async () => {
  let finishApply!: (value: ReturnType<typeof response>) => void;
  mockApply.mockReturnValue(
    new Promise((resolve) => {
      finishApply = resolve;
    }),
  );
  setup();
  saveParserChanges();
  const dialog = await screen.findByRole('alertdialog');
  fireEvent.click(screen.getByRole('button', { name: 'common.yes' }));
  await waitFor(() => {
    expect(screen.getByRole('button', { name: 'common.yes' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'common.no' })).toBeDisabled();
  });
  fireEvent.keyDown(dialog, { key: 'Escape', code: 'Escape' });
  expect(screen.getByRole('alertdialog')).toBeInTheDocument();
  await act(async () => finishApply(response({ code: 0, data: {} })));
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument(),
  );
  expect(mockApply).toHaveBeenCalledTimes(1);
});
