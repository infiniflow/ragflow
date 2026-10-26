import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { ChangeParserDialog } from './change-parser-dialog';
import { useChangeDocumentParser } from './use-change-document-parser';

const mockSave = jest.fn().mockResolvedValue(0);
const mockRun = jest.fn().mockResolvedValue(0);
const mockDsl = {
  graph: {
    nodes: [
      { id: 'File', data: { form: {} } },
      {
        id: 'Parser:parse',
        data: {
          form: { setups: [{ fileFormat: 'pdf', parse_method: 'deepdoc' }] },
        },
      },
      {
        id: 'Compiler:compile',
        data: {
          form: {
            compilation_template_group_id: 'pipeline-template-group',
            llm_id: 'pipeline-model',
          },
        },
      },
    ],
    edges: [
      { source: 'File', target: 'Parser:parse' },
      { source: 'Parser:parse', target: 'Compiler:compile' },
    ],
  },
};
const mockBuiltinDsl =
  require('../../../../../internal/ingestion/pipeline/template/ingestion_pipeline_general.json').dsl;
let mockCustomDsl = mockDsl;

jest.mock('@/hooks/use-agent-request', () => ({
  useFetchPipelineDslByPipelineId: (_id: string, isBuiltin: boolean) => ({
    dsl: isBuiltin ? mockBuiltinDsl : mockCustomDsl,
    loading: false,
  }),
}));
jest.mock('@/hooks/use-document-request', () => ({
  useRunDocument: () => ({ runDocumentByIds: mockRun, loading: false }),
  useSetDocumentPipelineParser: () => ({
    setDocumentPipelineParser: mockSave,
    loading: false,
  }),
}));
jest.mock('@/components/parser-select', () => ({
  ParserSelect: ({ onChange }: any) => {
    const choosePipeline = () => onChange('pipeline', 'pipeline');
    const chooseBuiltin = () => onChange('builtin', 'general');
    const clearParser = () => onChange(null, '');
    return (
      <>
        <button type="button" onClick={choosePipeline}>
          Choose pipeline
        </button>
        <button type="button" onClick={chooseBuiltin}>
          Choose builtin
        </button>
        <button type="button" onClick={clearParser}>
          Clear parser
        </button>
      </>
    );
  },
}));
jest.mock('@/routes', () => ({ Routes: {} }));
jest.mock('@/components/pipeline-operator-tabs/pipeline-operator-form', () => {
  const { useForm } = require('react-hook-form');
  const {
    useFormChangeCallback,
  } = require('@/pages/agent/hooks/use-form-change-callback');
  const { FormWrapper } = require('@/pages/agent/form/components/form-wrapper');
  return {
    __esModule: true,
    default: ({ node, onValuesChange }: any) => {
      const form = useForm({ defaultValues: node.data.form });
      useFormChangeCallback(form, onValuesChange);
      return (
        <FormWrapper>
          <input
            aria-label={node.data.operatorId}
            {...form.register('llm_id')}
          />
        </FormWrapper>
      );
    },
  };
});
jest.mock('react-i18next', () => ({
  ...jest.requireActual('react-i18next'),
  useTranslation: () => ({ t: (key: string) => key }),
}));

function Harness({
  builtin = false,
  chunkCount = 0,
}: {
  builtin?: boolean;
  chunkCount?: number;
}) {
  const state = useChangeDocumentParser();
  const openDialog = () => {
    state.showChangeParserModal({
      id: 'document',
      dataset_id: 'dataset',
      chunk_count: chunkCount,
      chunk_method: builtin ? 'general' : '',
      pipeline_id: builtin ? '' : 'pipeline',
      parser_config: builtin
        ? {}
        : {
            'Parser:parse': { pdf: { parse_method: 'deepdoc' } },
            'Compiler:compile': {
              compilation_template_group_id: 'pipeline-template-group',
              llm_id: 'document-model',
            },
          },
    } as any);
  };
  return (
    <>
      <button onClick={openDialog}>Open</button>
      {state.changeParserVisible && (
        <ChangeParserDialog
          record={state.changeParserRecord}
          onOk={state.onChangeParserOk}
          hideModal={state.hideChangeParserModal}
          loading={state.changeParserLoading}
        />
      )}
    </>
  );
}

beforeEach(() => {
  mockSave.mockReset().mockResolvedValue(0);
  mockRun.mockReset().mockResolvedValue(0);
  mockCustomDsl = mockDsl;
});

it('keeps existing chunks when re-parsing is declined after saving', async () => {
  render(<Harness builtin chunkCount={5} />);
  fireEvent.click(screen.getByRole('button', { name: 'Open' }));
  fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
  expect(
    await screen.findByText('knowledgeDetails.parserConfigReparseTip'),
  ).toBeInTheDocument();
  expect(mockSave).toHaveBeenCalledTimes(1);
  expect(mockRun).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'common.no' }));
  await waitFor(() =>
    expect(
      screen.queryByText('knowledgeDetails.parserConfigReparseTip'),
    ).not.toBeInTheDocument(),
  );
  expect(mockRun).not.toHaveBeenCalled();
});

it('re-parses with the saved document settings only after confirmation', async () => {
  render(<Harness chunkCount={5} />);
  fireEvent.click(screen.getByRole('button', { name: 'Open' }));
  fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
  const confirm = await screen.findByRole('button', { name: 'common.yes' });
  expect(mockSave).toHaveBeenCalledTimes(1);
  expect(mockRun).not.toHaveBeenCalled();
  fireEvent.click(confirm);
  await waitFor(() => expect(mockRun).toHaveBeenCalledTimes(1));
  expect(mockRun).toHaveBeenCalledWith({
    documentIds: ['document'],
    run: 1,
    option: { delete: true, apply_kb: false },
  });
});

it('does not offer re-parsing when saving fails', async () => {
  mockSave.mockResolvedValueOnce(102);
  render(<Harness builtin chunkCount={5} />);
  fireEvent.click(screen.getByRole('button', { name: 'Open' }));
  fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
  await waitFor(() => expect(mockSave).toHaveBeenCalledTimes(1));
  expect(
    screen.queryByText('knowledgeDetails.parserConfigReparseTip'),
  ).not.toBeInTheDocument();
  expect(
    screen.getByRole('button', { name: 'common.save' }),
  ).toBeInTheDocument();
  expect(mockRun).not.toHaveBeenCalled();
});

it('saves the built-in parser', async () => {
  render(<Harness builtin />);
  fireEvent.click(screen.getByRole('button', { name: 'Open' }));
  fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
  await waitFor(() => expect(mockSave).toHaveBeenCalledTimes(1));
  expect(mockSave.mock.calls[0][0]).toEqual(
    expect.objectContaining({
      parserId: 'general',
      pipelineId: '',
      parse_type: 1,
    }),
  );
  expect(
    screen.queryByText('knowledgeDetails.parserConfigReparseTip'),
  ).not.toBeInTheDocument();
  expect(mockRun).not.toHaveBeenCalled();
});

it('saves after switching from a built-in parser to a pipeline', async () => {
  render(<Harness builtin />);
  fireEvent.click(screen.getByRole('button', { name: 'Open' }));
  fireEvent.click(screen.getByRole('button', { name: 'Choose pipeline' }));
  fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
  await waitFor(() => expect(mockSave).toHaveBeenCalledTimes(1));
  expect(mockSave.mock.calls[0][0]).toEqual(
    expect.objectContaining({
      parserId: '',
      pipelineId: 'pipeline',
      parse_type: 2,
    }),
  );
});

it('saves the repository advanced ingestion pipeline template', async () => {
  mockCustomDsl =
    require('../../../../../internal/agent/templates/advanced_ingestion_pipeline.json').dsl;
  render(<Harness builtin />);
  fireEvent.click(screen.getByRole('button', { name: 'Open' }));
  fireEvent.click(screen.getByRole('button', { name: 'Choose pipeline' }));
  fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
  await waitFor(() => expect(mockSave).toHaveBeenCalledTimes(1));
  expect(mockSave.mock.calls[0][0].pipelineId).toBe('pipeline');
});

it('saves after switching from a pipeline back to a built-in parser', async () => {
  render(<Harness />);
  fireEvent.click(screen.getByRole('button', { name: 'Open' }));
  fireEvent.click(screen.getByRole('button', { name: 'Choose builtin' }));
  fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
  await waitFor(() => expect(mockSave).toHaveBeenCalledTimes(1));
  expect(mockSave.mock.calls[0][0]).toEqual(
    expect.objectContaining({
      parserId: 'general',
      pipelineId: '',
      parse_type: 1,
    }),
  );
});

it('still rejects a pipeline selection without a pipeline ID', async () => {
  render(<Harness builtin />);
  fireEvent.click(screen.getByRole('button', { name: 'Open' }));
  fireEvent.click(screen.getByRole('button', { name: 'Choose pipeline' }));
  fireEvent.click(screen.getByRole('button', { name: 'Clear parser' }));
  fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
  expect(await screen.findByText('common.pleaseSelect')).toBeInTheDocument();
  expect(mockSave).not.toHaveBeenCalled();
});
