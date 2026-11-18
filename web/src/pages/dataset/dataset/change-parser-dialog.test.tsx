import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import {
  ChangeParserDialog,
  ReparseConfirmDialog,
} from './change-parser-dialog';
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
// Per-version DSLs for the mocked pipeline; versions not listed here fall
// back to mockCustomDsl so existing single-version tests keep working.
let mockCustomDslByVersion: Record<string, typeof mockDsl> = {};
// Published versions of the mocked pipeline; a test can empty this to model a
// pinned version that has been pruned since the document was saved.
let mockVersions: { id: string; title: string; created_at: string }[] = [
  { id: 'version-1', title: 'v1', created_at: '' },
];

jest.mock('@/hooks/use-agent-request', () => ({
  useFetchPipelineDslByPipelineId: (
    _id: string,
    versionId: string | undefined,
    isBuiltin: boolean,
  ) => ({
    dsl: isBuiltin
      ? mockBuiltinDsl
      : (mockCustomDslByVersion[versionId ?? ''] ?? mockCustomDsl),
    loading: false,
  }),
  // The mocked ParserTreeSelect below emits version-1 on every pipeline
  // pick, so the canvas version list must contain it — otherwise the dialog
  // treats the selection as missing and form validation rejects the save.
  // An undefined canvas id models the hook's enabled gate (no fetch outside
  // pipeline mode).
  useFetchCanvasVersionList: (canvasId?: string) => ({
    data: canvasId ? mockVersions : [],
    loading: false,
    isError: false,
  }),
  // The three hooks below back useParserTreeData, which the form hook now
  // consults for the missing-option validation.
  useFetchBuiltinPipelines: () => ({
    options: [{ label: 'General', value: 'general' }],
    loading: false,
  }),
  useFetchAgentList: () => ({
    data: {
      canvas: [{ id: 'pipeline', title: 'Pipeline', type: 'pipeline' }],
      total: 1,
    },
    loading: false,
  }),
  useFetchCanvasVersions: (canvasIds: string[]) =>
    canvasIds.map(() => ({ isSuccess: true, data: mockVersions })),
}));
jest.mock('@/hooks/use-document-request', () => ({
  DocumentKeys: { all: () => ['documents'] },
  useRunDocument: () => ({ runDocumentByIds: mockRun, loading: false }),
  useSetDocumentPipelineParser: () => ({
    setDocumentPipelineParser: mockSave,
    loading: false,
  }),
}));
jest.mock('@/components/parser-tree-select', () => ({
  ParserTreeSelect: ({ onChange }: any) => {
    // A pipeline-side choice always carries a concrete version id: only
    // version leaves are selectable in the merged parser tree select.
    const choosePipeline = () => onChange('pipeline', 'pipeline', 'version-1');
    const choosePipelineV2 = () =>
      onChange('pipeline', 'pipeline', 'version-2');
    const chooseBuiltin = () => onChange('builtin', 'general');
    const clearParser = () => onChange(null, '');
    return (
      <>
        <button type="button" onClick={choosePipeline}>
          Choose pipeline
        </button>
        <button type="button" onClick={choosePipelineV2}>
          Choose pipeline v2
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
// The real module drags the compilation-template graph (markdown editor →
// mermaid/lexical ESM) into jsdom; these tests never exercise parser_config
// validation.
jest.mock(
  '@/components/pipeline-operator-tabs/parser-config-validation',
  () => ({
    addParserConfigIssues: () => {},
  }),
);
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
  versionId = '',
}: {
  builtin?: boolean;
  chunkCount?: number;
  versionId?: string;
}) {
  const state = useChangeDocumentParser();
  const openDialog = () => {
    state.showChangeParserModal({
      id: 'document',
      dataset_id: 'dataset',
      chunk_count: chunkCount,
      chunk_method: builtin ? 'general' : '',
      pipeline_id: builtin ? '' : 'pipeline',
      pipeline_version_id: versionId,
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
      {state.reparseVisible && (
        <ReparseConfirmDialog
          visible={state.reparseVisible}
          onOk={state.confirmReparse}
          onCancel={state.hideReparseModal}
        />
      )}
    </>
  );
}

beforeEach(() => {
  mockSave.mockReset().mockResolvedValue(0);
  mockRun.mockReset().mockResolvedValue(0);
  mockCustomDsl = mockDsl;
  mockCustomDslByVersion = {};
  mockVersions = [{ id: 'version-1', title: 'v1', created_at: '' }];
});

// useChangeDocumentParser (and the dialog's data hooks) need a query client;
// retry: false keeps a failed fetch from re-firing the global notification.
function renderHarness(props: {
  builtin?: boolean;
  chunkCount?: number;
  versionId?: string;
}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <Harness {...props} />
    </QueryClientProvider>,
  );
}

it('keeps existing chunks when re-parsing is declined after saving', async () => {
  renderHarness({ builtin: true, chunkCount: 5 });
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
  renderHarness({ chunkCount: 5 });
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
  renderHarness({ builtin: true, chunkCount: 5 });
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
  renderHarness({ builtin: true });
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
  renderHarness({ builtin: true });
  fireEvent.click(screen.getByRole('button', { name: 'Open' }));
  fireEvent.click(screen.getByRole('button', { name: 'Choose pipeline' }));
  fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
  await waitFor(() => expect(mockSave).toHaveBeenCalledTimes(1));
  expect(mockSave.mock.calls[0][0]).toEqual(
    expect.objectContaining({
      parserId: '',
      pipelineId: 'pipeline',
      pipelineVersionId: 'version-1',
      parse_type: 2,
    }),
  );
});

it('saves the repository advanced ingestion pipeline template', async () => {
  mockCustomDsl =
    require('../../../../../internal/agent/templates/advanced_ingestion_pipeline.json').dsl;
  renderHarness({ builtin: true });
  fireEvent.click(screen.getByRole('button', { name: 'Open' }));
  fireEvent.click(screen.getByRole('button', { name: 'Choose pipeline' }));
  fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
  await waitFor(() => expect(mockSave).toHaveBeenCalledTimes(1));
  expect(mockSave.mock.calls[0][0].pipelineId).toBe('pipeline');
});

it('saves after switching from a pipeline back to a built-in parser', async () => {
  renderHarness({});
  fireEvent.click(screen.getByRole('button', { name: 'Open' }));
  fireEvent.click(screen.getByRole('button', { name: 'Choose builtin' }));
  fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
  await waitFor(() => expect(mockSave).toHaveBeenCalledTimes(1));
  expect(mockSave.mock.calls[0][0]).toEqual(
    expect.objectContaining({
      parserId: 'general',
      pipelineId: '',
      pipelineVersionId: '',
      parse_type: 1,
    }),
  );
});

it('still rejects a pipeline selection without a pipeline ID', async () => {
  renderHarness({ builtin: true });
  fireEvent.click(screen.getByRole('button', { name: 'Open' }));
  fireEvent.click(screen.getByRole('button', { name: 'Choose pipeline' }));
  fireEvent.click(screen.getByRole('button', { name: 'Clear parser' }));
  fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
  expect(await screen.findByText('common.pleaseSelect')).toBeInTheDocument();
  expect(mockSave).not.toHaveBeenCalled();
});

it('resets parser_config to the new version defaults after a version switch', async () => {
  mockVersions = [
    { id: 'version-2', title: 'v2', created_at: '' },
    { id: 'version-1', title: 'v1', created_at: '' },
  ];
  mockCustomDslByVersion = {
    'version-1': mockDsl,
    'version-2': {
      graph: {
        nodes: [
          { id: 'File', data: { form: {} } },
          {
            id: 'Compiler:compile',
            data: {
              form: {
                compilation_template_group_id: 'v2-template-group',
                llm_id: 'v2-model',
              },
            },
          },
        ],
        edges: [{ source: 'File', target: 'Compiler:compile' }],
      },
    },
  };
  renderHarness({ versionId: 'version-1' });
  fireEvent.click(screen.getByRole('button', { name: 'Open' }));
  fireEvent.click(screen.getByRole('button', { name: 'Choose pipeline v2' }));
  fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
  await waitFor(() => expect(mockSave).toHaveBeenCalledTimes(1));
  expect(mockSave.mock.calls[0][0]).toEqual(
    expect.objectContaining({
      pipelineId: 'pipeline',
      pipelineVersionId: 'version-2',
      parserConfig: expect.objectContaining({
        // The saved 'document-model' pin belongs to version-1; after the
        // switch only version-2's DSL defaults remain.
        'Compiler:compile': expect.objectContaining({
          compilation_template_group_id: 'v2-template-group',
          llm_id: 'v2-model',
        }),
      }),
    }),
  );
});

it('keeps the saved parser_config across the latest-version prefill', async () => {
  // Unpinned document: the form opens with an empty version and the prefill
  // adopts the latest one — the saved config must survive intact.
  renderHarness({});
  fireEvent.click(screen.getByRole('button', { name: 'Open' }));
  fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
  await waitFor(() => expect(mockSave).toHaveBeenCalledTimes(1));
  expect(mockSave.mock.calls[0][0]).toEqual(
    expect.objectContaining({
      pipelineId: 'pipeline',
      pipelineVersionId: 'version-1',
      parserConfig: expect.objectContaining({
        'Compiler:compile': expect.objectContaining({
          llm_id: 'document-model',
        }),
      }),
    }),
  );
});

it('blocks saving when the pinned pipeline version no longer exists', async () => {
  mockVersions = [];
  renderHarness({ versionId: 'version-1' });
  fireEvent.click(screen.getByRole('button', { name: 'Open' }));

  // The missing verdict derives from the loaded parser tree, so the
  // unavailable message shows as soon as the dialog opens, without waiting
  // for a save attempt.
  expect(
    await screen.findByText(
      'knowledgeConfiguration.parserOptionUnavailableMessage',
    ),
  ).toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
  await waitFor(() =>
    expect(
      screen.getByText('knowledgeConfiguration.parserOptionUnavailableMessage'),
    ).toBeInTheDocument(),
  );
  expect(mockSave).not.toHaveBeenCalled();
});
