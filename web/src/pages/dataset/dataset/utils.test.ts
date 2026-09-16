import { FileType } from '@/constants/file';
import {
  IngestionTaskStatus,
  ParserModelKind,
  ParserGapReason,
  RunningStatus,
} from './constant';
import {
  findDocumentsParserGaps,
  findFilesParserGaps,
  findParserGap,
  getDocumentRunningStatus,
  getFileTypeByExtension,
  getSavedParserSetups,
  hasUnsupportedTypeGap,
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

// Saved setups shaped like a pipeline that declares pdf/image plus audio
// (model missing) and video (model configured).
const SavedSetups = [
  { fileFormat: FileType.PDF, parse_method: 'DeepDOC' },
  { fileFormat: FileType.Image, parse_method: 'ocr' },
  { fileFormat: FileType.Audio, vlm: { llm_id: '' } },
  { fileFormat: FileType.Video, vlm: { llm_id: 'gpt-4o@OpenAI' } },
] as Record<string, any>[];

describe('findParserGap', () => {
  it('flags a file type missing from the saved setups as unsupported', () => {
    expect(findParserGap(FileType.Docx, SavedSetups)).toEqual({
      reason: ParserGapReason.UnsupportedType,
      fileType: FileType.Docx,
    });
  });

  it('flags audio/video without a declared family as unsupported, not missing-model', () => {
    const setups = [{ fileFormat: FileType.PDF }];
    expect(findParserGap(FileType.Audio, setups)).toEqual({
      reason: ParserGapReason.UnsupportedType,
      fileType: FileType.Audio,
    });
    expect(findParserGap(FileType.Video, setups)).toEqual({
      reason: ParserGapReason.UnsupportedType,
      fileType: FileType.Video,
    });
  });

  it('flags declared audio without a model as a missing asr model', () => {
    expect(findParserGap(FileType.Audio, SavedSetups)).toEqual({
      reason: ParserGapReason.MissingModel,
      fileType: FileType.Audio,
      modelKind: ParserModelKind.Asr,
    });
  });

  it('flags declared video without a model as a missing vision model', () => {
    const setups = [{ fileFormat: FileType.Video, vlm: { llm_id: '' } }];
    expect(findParserGap(FileType.Video, setups)).toEqual({
      reason: ParserGapReason.MissingModel,
      fileType: FileType.Video,
      modelKind: ParserModelKind.Vision,
    });
  });

  it('passes video when the setup carries a model', () => {
    expect(findParserGap(FileType.Video, SavedSetups)).toBeNull();
  });

  it('passes image with an ocr-only setup — no model required', () => {
    expect(findParserGap(FileType.Image, SavedSetups)).toBeNull();
  });

  it('passes supported non-media types and unknown file types', () => {
    expect(findParserGap(FileType.PDF, SavedSetups)).toBeNull();
    expect(findParserGap(undefined, SavedSetups)).toBeNull();
  });
});

describe('findFilesParserGaps', () => {
  it('returns a gap per failing file, mixing both reasons', () => {
    const gaps = findFilesParserGaps(
      ['song.mp3', 'notes.docx', 'photo.jpg', 'movie.mp4'],
      SavedSetups,
    );
    expect(gaps).toEqual([
      {
        name: 'song.mp3',
        reason: ParserGapReason.MissingModel,
        fileType: FileType.Audio,
        modelKind: ParserModelKind.Asr,
      },
      {
        name: 'notes.docx',
        reason: ParserGapReason.UnsupportedType,
        fileType: FileType.Docx,
      },
    ]);
  });
});

describe('hasUnsupportedTypeGap', () => {
  it('detects unsupported-type gaps among mixed gaps', () => {
    expect(
      hasUnsupportedTypeGap([
        {
          reason: ParserGapReason.MissingModel,
          fileType: FileType.Audio,
          modelKind: ParserModelKind.Asr,
        },
        { reason: ParserGapReason.UnsupportedType, fileType: FileType.Image },
      ]),
    ).toBe(true);
    expect(
      hasUnsupportedTypeGap([
        {
          reason: ParserGapReason.MissingModel,
          fileType: FileType.Audio,
          modelKind: ParserModelKind.Asr,
        },
      ]),
    ).toBe(false);
    expect(hasUnsupportedTypeGap([])).toBe(false);
  });
});

describe('getSavedParserSetups', () => {
  it('returns null without a saved parser config', () => {
    expect(getSavedParserSetups(null)).toBeNull();
    expect(
      getSavedParserSetups({
        parser_config: {},
      }),
    ).toBeNull();
  });

  it('returns null for legacy flat-shape configs without a Parser entry', () => {
    const knowledgeBase = {
      parser_config: { chunk_token_num: 128, delimiter: '\n' },
    };
    expect(getSavedParserSetups(knowledgeBase)).toBeNull();
  });

  it('returns only the saved Parser setups, without merging defaults', () => {
    const knowledgeBase = {
      parser_config: {
        'Parser:1': {
          pdf: { parse_method: 'DeepDOC' },
          image: { parse_method: 'ocr' },
        },
      },
    };
    const setups = getSavedParserSetups(knowledgeBase);
    expect(setups?.map((x) => x.fileFormat).sort()).toEqual(['image', 'pdf']);
  });

  it('ignores non-parser operator entries', () => {
    const knowledgeBase = {
      parser_config: {
        'TokenChunker:1': { chunk_token_size: 128 },
      },
    };
    expect(getSavedParserSetups(knowledgeBase)).toBeNull();
  });
});

describe('findDocumentsParserGaps', () => {
  // A document overridden to a general-like parser: its row parser_config
  // declares pdf even though the dataset-level fallback does not.
  const OverriddenDoc = {
    name: 'civil-code.pdf',
    parser_config: {
      'Parser:1': { pdf: { parse_method: 'DeepDOC' } },
    },
  };

  it('validates against the document row config when present', () => {
    expect(findDocumentsParserGaps([OverriddenDoc], null)).toEqual([]);
  });

  it('falls back to the dataset-level setups for rows without a Parser entry', () => {
    const legacyRow = { name: 'song.mp3', parser_config: {} };
    expect(findDocumentsParserGaps([legacyRow], SavedSetups)).toEqual([
      {
        name: 'song.mp3',
        reason: ParserGapReason.MissingModel,
        fileType: FileType.Audio,
        modelKind: ParserModelKind.Asr,
      },
    ]);
    // No fallback either: nothing can be determined, so nothing is flagged.
    expect(findDocumentsParserGaps([legacyRow], null)).toEqual([]);
  });

  it('evaluates each document against its own config in a mixed batch', () => {
    const inheritedRow = { name: 'photo.jpg', parser_config: undefined };
    const gaps = findDocumentsParserGaps(
      [OverriddenDoc, inheritedRow],
      // Dataset-level config declares only audio: the overridden pdf passes
      // while the inherited jpg is flagged.
      [{ fileFormat: FileType.Audio, vlm: { llm_id: 'whisper@OpenAI' } }],
    );
    expect(gaps).toEqual([
      {
        name: 'photo.jpg',
        reason: ParserGapReason.UnsupportedType,
        fileType: FileType.Image,
      },
    ]);
  });
});
