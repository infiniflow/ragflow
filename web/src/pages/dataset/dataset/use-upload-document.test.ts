import { act, renderHook, waitFor } from '@testing-library/react';

// NOTE: this suite uses jest.mock, so it is transformed by esbuild-jest's
// babel pipeline, which does not strip TypeScript. Imported bindings must not
// appear in type annotations, so shapes are declared locally and values are
// cast at the call site.
type Doc = {
  id: string;
  name: string;
  parser_config?: Record<string, unknown>;
  [key: string]: unknown;
};

const mockUpload = jest.fn();
const mockRun = jest.fn();
const mockWarning = jest.fn();

// Per-test gap fixtures: pre-upload gaps by file name, and post-upload
// document gaps keyed by document name. The real gap detection is covered by
// utils.test.ts; here both validators are stubbed so the wiring under test is
// which documents the hook auto-parses.
let mockUploadGaps: Array<Record<string, unknown>> = [];
let mockDocumentGaps: Record<string, Array<Record<string, unknown>>> = {};
let mockPipelineMissing = false;
// The warning mock auto-confirms by default so the pre-existing gap tests
// keep flowing into the upload; cancellation tests flip this off.
let mockAutoConfirmWarning = true;

jest.mock('./use-dataset-pipeline-missing', () => ({
  useDatasetPipelineMissing: () => mockPipelineMissing,
}));

jest.mock('@/hooks/use-document-request', () => ({
  useUploadDocument: () => ({ uploadDocument: mockUpload, loading: false }),
  useRunDocument: () => ({ runDocumentByIds: mockRun, loading: false }),
}));

jest.mock('./use-parser-gap-validation', () => ({
  useParserGapValidation: () => ({
    findParseGaps: (names: string[]) =>
      mockUploadGaps.filter((gap) => names.includes(gap.name as string)),
    findDocumentParseGaps: (docs: Doc[]) =>
      docs.flatMap((doc) => mockDocumentGaps[doc.name] ?? []),
  }),
}));

jest.mock('@/components/ui/modal/modal', () => ({
  Modal: {
    warning: (options: { onOk?: () => void }) => {
      mockWarning(options);
      if (mockAutoConfirmWarning) {
        options.onOk?.();
      }
    },
  },
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

import { useHandleUploadDocument } from './use-upload-document';

function makeDoc(overrides: Partial<Doc> = {}): Doc {
  return { id: 'doc-1', name: 'photo.jpg', parser_config: {}, ...overrides };
}

function uploadOkResponse(docs: Doc[]) {
  return { code: 0, data: docs };
}

beforeEach(() => {
  jest.clearAllMocks();
  mockUploadGaps = [];
  mockDocumentGaps = {};
  mockPipelineMissing = false;
  mockAutoConfirmWarning = true;
  mockUpload.mockResolvedValue(uploadOkResponse([makeDoc()]));
});

// The regression case for the auto-parse bug: an unsupported file type (here
// a presentation) is reseeded by the backend with its dedicated pipeline, and
// the uploaded response carries the converted parser_config. The pre-upload gap
// prediction must not exclude the document from auto-parse anymore —
// runnability follows the document's actual config.
it('auto-parses a document the backend switched to a dedicated parser', async () => {
  mockUploadGaps = [
    { reason: 'unsupportedType', name: 'deck.pptx', fileType: 'PowerPoint' },
  ];
  mockUpload.mockResolvedValue(
    uploadOkResponse([
      makeDoc({
        id: 'doc-deck',
        name: 'deck.pptx',
        parser_config: {
          'Parser:parse': { setups: [{ fileFormat: 'slides' }] },
        },
      }),
    ]),
  );

  const { result } = renderHook(() => useHandleUploadDocument());
  await act(async () => {
    await result.current.onDocumentUploadOk({
      fileList: [new File(['x'], 'deck.pptx')],
      parseOnCreation: true,
    });
  });

  expect(mockWarning).toHaveBeenCalledTimes(1);
  await waitFor(() => expect(mockRun).toHaveBeenCalledTimes(1));
  expect(mockRun).toHaveBeenCalledWith({
    documentIds: ['doc-deck'],
    run: 1,
  });
});

it('skips auto-parse when the uploaded config still has a gap', async () => {
  mockUploadGaps = [
    { reason: 'missingModel', name: 'song.mp3', fileType: 'Audio' },
  ];
  mockUpload.mockResolvedValue(
    uploadOkResponse([makeDoc({ id: 'doc-audio', name: 'song.mp3' })]),
  );
  mockDocumentGaps = {
    'song.mp3': [
      { reason: 'missingModel', name: 'song.mp3', fileType: 'Audio' },
    ],
  };

  const { result } = renderHook(() => useHandleUploadDocument());
  await act(async () => {
    await result.current.onDocumentUploadOk({
      fileList: [new File(['x'], 'song.mp3')],
      parseOnCreation: true,
    });
  });

  await waitFor(() => expect(mockUpload).toHaveBeenCalledTimes(1));
  // Give the floating proceedUpload promise a chance to settle before
  // asserting it did not schedule a run.
  await act(async () => {
    await Promise.resolve();
  });
  expect(mockRun).not.toHaveBeenCalled();
});

it('auto-parses only the runnable documents of a mixed batch', async () => {
  mockUpload.mockResolvedValue(
    uploadOkResponse([
      makeDoc({ id: 'doc-pdf', name: 'a.pdf' }),
      makeDoc({ id: 'doc-audio', name: 'song.mp3' }),
    ]),
  );
  mockDocumentGaps = {
    'song.mp3': [
      { reason: 'missingModel', name: 'song.mp3', fileType: 'Audio' },
    ],
  };

  const { result } = renderHook(() => useHandleUploadDocument());
  await act(async () => {
    await result.current.onDocumentUploadOk({
      fileList: [new File(['x'], 'a.pdf'), new File(['x'], 'song.mp3')],
      parseOnCreation: true,
    });
  });

  await waitFor(() => expect(mockRun).toHaveBeenCalledTimes(1));
  expect(mockRun).toHaveBeenCalledWith({
    documentIds: ['doc-pdf'],
    run: 1,
  });
});

it('does not run when auto-parse is unchecked', async () => {
  const { result } = renderHook(() => useHandleUploadDocument());
  await act(async () => {
    await result.current.onDocumentUploadOk({
      fileList: [new File(['x'], 'a.pdf')],
      parseOnCreation: false,
    });
  });

  await waitFor(() => expect(mockUpload).toHaveBeenCalledTimes(1));
  await act(async () => {
    await Promise.resolve();
  });
  expect(mockRun).not.toHaveBeenCalled();
});

it('does not run when the upload fails', async () => {
  mockUpload.mockResolvedValue({ code: 123, message: 'boom' });

  const { result } = renderHook(() => useHandleUploadDocument());
  await act(async () => {
    await result.current.onDocumentUploadOk({
      fileList: [new File(['x'], 'a.pdf')],
      parseOnCreation: true,
    });
  });

  await waitFor(() => expect(mockUpload).toHaveBeenCalledTimes(1));
  await act(async () => {
    await Promise.resolve();
  });
  expect(mockRun).not.toHaveBeenCalled();
});

it('auto-parses every document on the no-gap path without a warning modal', async () => {
  mockUpload.mockResolvedValue(
    uploadOkResponse([
      makeDoc({ id: 'doc-a', name: 'a.pdf' }),
      makeDoc({ id: 'doc-b', name: 'b.docx' }),
    ]),
  );

  const { result } = renderHook(() => useHandleUploadDocument());
  await act(async () => {
    await result.current.onDocumentUploadOk({
      fileList: [new File(['x'], 'a.pdf'), new File(['x'], 'b.docx')],
      parseOnCreation: true,
    });
  });

  expect(mockWarning).not.toHaveBeenCalled();
  await waitFor(() => expect(mockRun).toHaveBeenCalledTimes(1));
  expect(mockRun).toHaveBeenCalledWith({
    documentIds: ['doc-a', 'doc-b'],
    run: 1,
  });
});

// When the dataset's configured pipeline is gone, parse-on-creation would
// silently parse nothing, so the hook confirms first and drops the auto-parse
// on "Continue upload".
it('warns and uploads without parsing when the dataset pipeline is missing', async () => {
  mockPipelineMissing = true;

  const { result } = renderHook(() => useHandleUploadDocument('canvas-1'));
  await act(async () => {
    await result.current.onDocumentUploadOk({
      fileList: [new File(['x'], 'a.pdf')],
      parseOnCreation: true,
    });
  });

  expect(mockWarning).toHaveBeenCalledTimes(1);
  expect(mockWarning).toHaveBeenCalledWith(
    expect.objectContaining({
      title: 'knowledgeDetails.uploadPipelineDeletedTitle',
      content: 'knowledgeDetails.uploadPipelineDeletedContent',
    }),
  );
  await waitFor(() => expect(mockUpload).toHaveBeenCalledTimes(1));
  await act(async () => {
    await Promise.resolve();
  });
  expect(mockRun).not.toHaveBeenCalled();
});

it('uploads without a warning when parse-on-creation is off even if the pipeline is missing', async () => {
  mockPipelineMissing = true;

  const { result } = renderHook(() => useHandleUploadDocument('canvas-1'));
  await act(async () => {
    await result.current.onDocumentUploadOk({
      fileList: [new File(['x'], 'a.pdf')],
      parseOnCreation: false,
    });
  });

  expect(mockWarning).not.toHaveBeenCalled();
  await waitFor(() => expect(mockUpload).toHaveBeenCalledTimes(1));
});

it('chains the gap warning after the pipeline warning, then uploads without parsing', async () => {
  mockPipelineMissing = true;
  mockUploadGaps = [
    { reason: 'unsupportedType', name: 'deck.pptx', fileType: 'PowerPoint' },
  ];
  mockUpload.mockResolvedValue(
    uploadOkResponse([makeDoc({ id: 'doc-deck', name: 'deck.pptx' })]),
  );

  const { result } = renderHook(() => useHandleUploadDocument('canvas-1'));
  await act(async () => {
    await result.current.onDocumentUploadOk({
      fileList: [new File(['x'], 'deck.pptx')],
      parseOnCreation: true,
    });
  });

  expect(mockWarning).toHaveBeenCalledTimes(2);
  expect(mockWarning.mock.calls[0][0].title).toBe(
    'knowledgeDetails.uploadPipelineDeletedTitle',
  );
  expect(mockWarning.mock.calls[1][0].title).toBe(
    'knowledgeDetails.uploadUnsupportedTypesTitle',
  );
  await waitFor(() => expect(mockUpload).toHaveBeenCalledTimes(1));
  await act(async () => {
    await Promise.resolve();
  });
  expect(mockRun).not.toHaveBeenCalled();
});

it('does not upload when the pipeline warning is cancelled', async () => {
  mockPipelineMissing = true;
  mockAutoConfirmWarning = false;

  const { result } = renderHook(() => useHandleUploadDocument('canvas-1'));
  await act(async () => {
    await result.current.onDocumentUploadOk({
      fileList: [new File(['x'], 'a.pdf')],
      parseOnCreation: true,
    });
  });

  expect(mockWarning).toHaveBeenCalledTimes(1);
  await act(async () => {
    await Promise.resolve();
  });
  expect(mockUpload).not.toHaveBeenCalled();
  expect(mockRun).not.toHaveBeenCalled();
});
