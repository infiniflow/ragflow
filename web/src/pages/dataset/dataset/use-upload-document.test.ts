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
      options.onOk?.();
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
  mockUpload.mockResolvedValue(uploadOkResponse([makeDoc()]));
});

// The regression case for the auto-parse bug: an unsupported file type (here
// an image) is reseeded by the backend with its dedicated pipeline, and the
// uploaded response carries the converted parser_config. The pre-upload gap
// prediction must not exclude the document from auto-parse anymore —
// runnability follows the document's actual config.
it('auto-parses a document the backend switched to a dedicated parser', async () => {
  mockUploadGaps = [
    { reason: 'unsupportedType', name: 'photo.jpg', fileType: 'Image' },
  ];
  mockUpload.mockResolvedValue(
    uploadOkResponse([
      makeDoc({
        id: 'doc-img',
        parser_config: {
          'Parser:parse': { setups: [{ fileFormat: 'Image' }] },
        },
      }),
    ]),
  );

  const { result } = renderHook(() => useHandleUploadDocument());
  await act(async () => {
    await result.current.onDocumentUploadOk({
      fileList: [new File(['x'], 'photo.jpg')],
      parseOnCreation: true,
    });
  });

  expect(mockWarning).toHaveBeenCalledTimes(1);
  await waitFor(() => expect(mockRun).toHaveBeenCalledTimes(1));
  expect(mockRun).toHaveBeenCalledWith({
    documentIds: ['doc-img'],
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
