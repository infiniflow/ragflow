import { IDataset } from '@/interfaces/database/dataset';
import { FileType, initialParserValues } from '@/pages/agent/constant/pipeline';
import { IngestionTaskStatus, RunningStatus } from './constant';
import {
  findFilesMissingParserModels,
  findMissingParserModel,
  getEffectiveParserSetups,
  getFileTypeByExtension,
  isDocumentProcessing,
  isParserRunning,
} from './utils';

describe('isParserRunning', () => {
  it.each([RunningStatus.RUNNING, RunningStatus.SCHEDULE])(
    'treats %s as active parsing',
    (status) => {
      expect(isParserRunning(status)).toBe(true);
    },
  );

  it.each([
    RunningStatus.UNSTART,
    RunningStatus.CANCEL,
    RunningStatus.DONE,
    RunningStatus.FAIL,
  ])('does not treat %s as active parsing', (status) => {
    expect(isParserRunning(status)).toBe(false);
  });
});

describe('isDocumentProcessing', () => {
  it('treats a scheduled ingestion task as processing before parsing starts', () => {
    expect(
      isDocumentProcessing({
        run: RunningStatus.UNSTART,
        ingestion_status: IngestionTaskStatus.SCHEDULED,
      }),
    ).toBe(true);
  });

  it.each([
    IngestionTaskStatus.CREATED,
    IngestionTaskStatus.RUNNING,
    IngestionTaskStatus.STOPPING,
  ])(
    'treats an active %s ingestion task as processing before document state sync',
    (ingestionStatus) => {
      expect(
        isDocumentProcessing({
          run: RunningStatus.UNSTART,
          ingestion_status: ingestionStatus,
        }),
      ).toBe(true);
    },
  );
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
    } as unknown as IDataset;
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
    } as unknown as IDataset;
    const setups = getEffectiveParserSetups(knowledgeBase);
    expect(setups).toHaveLength(initialParserValues.setups.length);
  });
});
