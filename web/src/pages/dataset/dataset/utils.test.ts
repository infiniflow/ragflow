import { FileType, initialParserValues } from '@/pages/agent/constant/pipeline';
import { IngestionTaskStatus, RunningStatus } from './constant';
import {
  findFilesMissingParserModels,
  findMissingParserModel,
  getDocumentRunningStatus,
  getEffectiveParserSetups,
  getFileTypeByExtension,
  ingestionStatusToRunningStatus,
  isDocumentProcessing,
  isDocumentStopping,
} from './utils';

// Dispatch on the mutable flag so each scenario can exercise either the
// Go or the Python branch without mounting React.
let mockIsGoBackend = false;
jest.mock('@/utils/backend-variant', () => ({
  pickByBackend: ({ go, python }: { go: unknown; python: unknown }) =>
    mockIsGoBackend ? go : python,
}));

describe('isDocumentStopping', () => {
  it('is true only for the Go STOPPING status', () => {
    expect(
      isDocumentStopping({ ingestion_status: IngestionTaskStatus.STOPPING }),
    ).toBe(true);
    expect(
      isDocumentStopping({ ingestion_status: IngestionTaskStatus.RUNNING }),
    ).toBe(false);
    expect(isDocumentStopping({ ingestion_status: undefined })).toBe(false);
  });
});

describe('isDocumentProcessing', () => {
  afterEach(() => {
    mockIsGoBackend = false;
  });

  describe('Go backend', () => {
    beforeEach(() => {
      mockIsGoBackend = true;
    });

    it.each([
      IngestionTaskStatus.CREATED,
      IngestionTaskStatus.SCHEDULED,
      IngestionTaskStatus.RUNNING,
      IngestionTaskStatus.STOPPING,
    ])('treats active ingestion_status %s as processing', (status) => {
      expect(isDocumentProcessing({ ingestion_status: status })).toBe(true);
    });

    it.each([
      IngestionTaskStatus.UNSTART,
      IngestionTaskStatus.COMPLETED,
      IngestionTaskStatus.FAILED,
      IngestionTaskStatus.STOPPED,
      undefined,
    ])(
      'treats idle/terminal ingestion_status %s as not processing',
      (status) => {
        expect(isDocumentProcessing({ ingestion_status: status })).toBe(false);
      },
    );

    it('ignores a stale legacy run field on Go responses', () => {
      expect(
        isDocumentProcessing({
          run: RunningStatus.RUNNING,
          ingestion_status: IngestionTaskStatus.COMPLETED,
        }),
      ).toBe(false);
    });
  });

  describe('Python backend', () => {
    beforeEach(() => {
      mockIsGoBackend = false;
    });

    it('treats run=RUNNING as processing', () => {
      expect(isDocumentProcessing({ run: RunningStatus.RUNNING })).toBe(true);
    });

    it.each([
      RunningStatus.UNSTART,
      RunningStatus.CANCEL,
      RunningStatus.DONE,
      RunningStatus.FAIL,
      undefined,
    ])('treats terminal/absent run %s as not processing', (run) => {
      expect(isDocumentProcessing({ run })).toBe(false);
    });

    // Python never serializes ingestion_status; even if a stray field
    // reached the client, it must not influence the run-based result.
    it('ignores ingestion_status because Python never sends it', () => {
      expect(
        isDocumentProcessing({
          run: RunningStatus.DONE,
          ingestion_status: IngestionTaskStatus.RUNNING,
        }),
      ).toBe(false);
      expect(
        isDocumentProcessing({
          run: RunningStatus.RUNNING,
          ingestion_status: IngestionTaskStatus.STOPPED,
        }),
      ).toBe(true);
    });
  });
});

describe('ingestionStatusToRunningStatus', () => {
  it.each([
    [IngestionTaskStatus.CREATED, RunningStatus.QUEUED],
    [IngestionTaskStatus.SCHEDULED, RunningStatus.QUEUED],
    [IngestionTaskStatus.RUNNING, RunningStatus.RUNNING],
    [IngestionTaskStatus.STOPPING, RunningStatus.RUNNING],
    [IngestionTaskStatus.COMPLETED, RunningStatus.DONE],
    [IngestionTaskStatus.FAILED, RunningStatus.FAIL],
    [IngestionTaskStatus.STOPPED, RunningStatus.CANCEL],
    [IngestionTaskStatus.UNSTART, RunningStatus.UNSTART],
    [undefined, RunningStatus.UNSTART],
  ] as const)('maps %s to %s', (status, expected) => {
    expect(ingestionStatusToRunningStatus(status)).toBe(expected);
  });
});

describe('getDocumentRunningStatus', () => {
  afterEach(() => {
    mockIsGoBackend = false;
  });

  it('derives every status from ingestion_status on Go', () => {
    mockIsGoBackend = true;
    expect(
      getDocumentRunningStatus({
        run: undefined,
        ingestion_status: IngestionTaskStatus.STOPPED,
      }),
    ).toBe(RunningStatus.CANCEL);
    expect(
      getDocumentRunningStatus({
        run: RunningStatus.RUNNING,
        ingestion_status: IngestionTaskStatus.COMPLETED,
      }),
    ).toBe(RunningStatus.DONE);
  });

  it('reads the legacy run field on Python', () => {
    expect(getDocumentRunningStatus({ run: RunningStatus.DONE })).toBe(
      RunningStatus.DONE,
    );
  });

  it('falls back to UNSTART when Python omits run', () => {
    expect(getDocumentRunningStatus({})).toBe(RunningStatus.UNSTART);
  });
});

describe('getFileTypeByExtension', () => {
  it('maps media extensions to their file types', () => {
    expect(getFileTypeByExtension('mp3')).toBe(FileType.Audio);
    expect(getFileTypeByExtension('wav')).toBe(FileType.Audio);
    expect(getFileTypeByExtension('mp4')).toBe(FileType.Video);
    expect(getFileTypeByExtension('jpeg')).toBe(FileType.Image);
    expect(getFileTypeByExtension('PNG')).toBe(FileType.Image);
    expect(getFileTypeByExtension('pdf')).toBe(FileType.PDF);
  });

  it('returns undefined for unknown extensions', () => {
    expect(getFileTypeByExtension('xyz')).toBeUndefined();
    expect(getFileTypeByExtension('')).toBeUndefined();
  });
});

// Minimal structural stand-in for IDataset in these tests. It is declared
// locally (instead of importing the interface) because the esbuild-jest
// babel hoisting pipeline cannot elide imported bindings used only in type
// positions.
type DatasetWithParserConfig = { parser_config: any };

const DefaultSetups = initialParserValues.setups as Record<string, any>[];

describe('findMissingParserModel', () => {
  it('passes audio when the setup carries a model', () => {
    const setups = [
      { fileFormat: FileType.Audio, vlm: { llm_id: 'whisper@OpenAI' } },
    ];
    expect(findMissingParserModel(FileType.Audio, setups)).toBeNull();
  });

  it('flags audio when the setup has no model, ignoring the tenant default', () => {
    expect(findMissingParserModel(FileType.Audio, DefaultSetups)).toEqual({
      fileType: FileType.Audio,
      modelKind: 'asr',
    });
  });

  it('passes video when the setup carries a model', () => {
    const setups = [
      { fileFormat: FileType.Video, vlm: { llm_id: 'gpt-4o@OpenAI' } },
    ];
    expect(findMissingParserModel(FileType.Video, setups)).toBeNull();
  });

  it('flags video when the setup has no model', () => {
    expect(findMissingParserModel(FileType.Video, DefaultSetups)).toEqual({
      fileType: FileType.Video,
      modelKind: 'vision',
    });
  });

  it('flags image when parse_method is a static method like ocr', () => {
    // The default image setup uses ocr — no vision model configured
    expect(findMissingParserModel(FileType.Image, DefaultSetups)).toEqual({
      fileType: FileType.Image,
      modelKind: 'vision',
    });
  });

  it('passes image when parse_method is a vision model', () => {
    const setups = [
      { fileFormat: FileType.Image, parse_method: 'gpt-4o@OpenAI' },
    ];
    expect(findMissingParserModel(FileType.Image, setups)).toBeNull();
  });

  it('ignores file types without model requirements', () => {
    expect(findMissingParserModel(FileType.PDF, DefaultSetups)).toBeNull();
    expect(findMissingParserModel(undefined, DefaultSetups)).toBeNull();
  });
});

describe('findFilesMissingParserModels', () => {
  it('returns a gap per file that lacks its required model', () => {
    const gaps = findFilesMissingParserModels(
      ['song.mp3', 'notes.txt', 'photo.jpg'],
      DefaultSetups,
    );
    expect(gaps).toEqual([
      { name: 'song.mp3', fileType: FileType.Audio, modelKind: 'asr' },
      { name: 'photo.jpg', fileType: FileType.Image, modelKind: 'vision' },
    ]);
  });
});

describe('getEffectiveParserSetups', () => {
  it('falls back to the default setups without a saved parser config', () => {
    const setups = getEffectiveParserSetups(null);
    expect(setups.map((x) => x.fileFormat)).toEqual(
      initialParserValues.setups.map((x) => x.fileFormat),
    );
  });

  it('applies saved overrides keyed by file format', () => {
    const knowledgeBase = {
      parser_config: {
        'Parser:1': {
          audio: { vlm: { llm_id: 'whisper@OpenAI' } },
        },
      },
    } as unknown as DatasetWithParserConfig;
    const setups = getEffectiveParserSetups(knowledgeBase);
    const audio = setups.find((x) => x.fileFormat === FileType.Audio);
    expect(audio?.vlm?.llm_id).toBe('whisper@OpenAI');
    // Untouched types keep their defaults
    const image = setups.find((x) => x.fileFormat === FileType.Image);
    expect(image?.parse_method).toBe('ocr');
  });

  it('ignores non-parser operator entries', () => {
    const knowledgeBase = {
      parser_config: {
        'TokenChunker:1': { chunk_token_size: 128 },
      },
    } as unknown as DatasetWithParserConfig;
    const setups = getEffectiveParserSetups(knowledgeBase);
    expect(setups).toHaveLength(initialParserValues.setups.length);
  });
});
