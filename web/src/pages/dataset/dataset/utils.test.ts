import { FileType } from '@/constants/file';
import { getExtension } from '@/utils/document-util';
import {
  IngestionTaskStatus,
  ParserModelKind,
  ParserGapReason,
  RunningStatus,
} from './constant';
import {
  attachAutoSwitchTargets,
  builtinParserSwitchTarget,
  findDocumentsParserGaps,
  findFilesParserGaps,
  findParserGap,
  getDocumentParserLabel,
  getDocumentRunningStatus,
  getDocumentProgressMessage,
  getFileTypeByExtension,
  getSavedParserConfig,
  hasUnsupportedTypeGap,
  ingestionStatusToRunningStatus,
  isDocumentProcessing,
  isDocumentStopping,
  pickBySwitchFate,
  type SavedParserConfig,
  type UploadFileParserGap,
} from './utils';

// Mirrors how findFilesParserGaps feeds the switch decision: the family comes
// from the extension map, the suffix sets from the backend's accepted owners.
const switchTarget = (name: string) =>
  builtinParserSwitchTarget(name, getFileTypeByExtension(getExtension(name)));

describe('isDocumentStopping', () => {
  it('is true only for the STOPPING status', () => {
    expect(
      isDocumentStopping({ ingestion_status: IngestionTaskStatus.STOPPING }),
    ).toBe(true);
    expect(
      isDocumentStopping({ ingestion_status: IngestionTaskStatus.RUNNING }),
    ).toBe(false);
    expect(isDocumentStopping({ ingestion_status: undefined })).toBe(false);
  });
});

describe('getDocumentParserLabel', () => {
  const builtin = (chunk_method: string) => ({
    pipeline_id: '',
    pipeline_name: '',
    chunk_method,
  });

  it('capitalizes the first letter of a built-in parser id', () => {
    expect(getDocumentParserLabel(builtin('paper'))).toBe('Paper');
    expect(getDocumentParserLabel(builtin('knowledge_graph'))).toBe(
      'Knowledge_graph',
    );
  });

  it('maps the legacy naive alias onto the general pipeline', () => {
    expect(getDocumentParserLabel(builtin('naive'))).toBe('General');
  });

  it('keeps a canvas pipeline title in its original format', () => {
    expect(
      getDocumentParserLabel({
        pipeline_id: 'abc123',
        pipeline_name: 'my custom pipeline',
        chunk_method: 'naive',
      }),
    ).toBe('my custom pipeline');
  });

  it('falls back to the raw pipeline id when the title is empty', () => {
    expect(
      getDocumentParserLabel({
        pipeline_id: 'abc123',
        pipeline_name: '',
        chunk_method: 'naive',
      }),
    ).toBe('abc123');
  });
});

describe('getDocumentProgressMessage', () => {
  it('reads the latest real ingestion event', () => {
    expect(
      getDocumentProgressMessage({
        latest_ingestion_event: {
          id: 42,
          ts: '2026-01-01T00:00:00Z',
          event_type: 1,
          component: '',
          phase: 0,
          message: 'Indexing 4/10',
        },
      }),
    ).toBe('Indexing 4/10');
  });

  it('returns a placeholder when the selected source has no message', () => {
    expect(getDocumentProgressMessage({})).toBe('-');
  });
});

describe('isDocumentProcessing', () => {
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
  ])('treats idle/terminal ingestion_status %s as not processing', (status) => {
    expect(isDocumentProcessing({ ingestion_status: status })).toBe(false);
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
  it('derives every status from ingestion_status', () => {
    expect(
      getDocumentRunningStatus({
        ingestion_status: IngestionTaskStatus.STOPPED,
      }),
    ).toBe(RunningStatus.CANCEL);
    expect(
      getDocumentRunningStatus({
        ingestion_status: IngestionTaskStatus.COMPLETED,
      }),
    ).toBe(RunningStatus.DONE);
  });

  it('falls back to UNSTART when status is missing', () => {
    expect(getDocumentRunningStatus({})).toBe(RunningStatus.UNSTART);
  });
});

describe('getFileTypeByExtension', () => {
  it('maps media extensions to their file types', () => {
    expect(getFileTypeByExtension('mp3')).toBe(FileType.Audio);
    expect(getFileTypeByExtension('wav')).toBe(FileType.Audio);
    expect(getFileTypeByExtension('mp4')).toBe(FileType.Video);
    expect(getFileTypeByExtension('mov')).toBe(FileType.Video);
    expect(getFileTypeByExtension('webm')).toBe(FileType.Video);
    expect(getFileTypeByExtension('flv')).toBe(FileType.Video);
    expect(getFileTypeByExtension('wmv')).toBe(FileType.Video);
    // Legacy containers share the same backend video set, so the upload warning
    // and parse block must recognize them too.
    expect(getFileTypeByExtension('rm')).toBe(FileType.Video);
    expect(getFileTypeByExtension('rmvb')).toBe(FileType.Video);
    expect(getFileTypeByExtension('asf')).toBe(FileType.Video);
    expect(getFileTypeByExtension('dat')).toBe(FileType.Video);
    expect(getFileTypeByExtension('asx')).toBe(FileType.Video);
    expect(getFileTypeByExtension('wvx')).toBe(FileType.Video);
    expect(getFileTypeByExtension('mpe')).toBe(FileType.Video);
    expect(getFileTypeByExtension('mpa')).toBe(FileType.Video);
    expect(getFileTypeByExtension('jpeg')).toBe(FileType.Image);
    expect(getFileTypeByExtension('PNG')).toBe(FileType.Image);
    expect(getFileTypeByExtension('pdf')).toBe(FileType.PDF);
  });

  it('ignores surrounding whitespace like the backend does', () => {
    // getExtension trims (mirroring utility.normalizeFilename); without it a
    // name like 'clip.mov ' maps to nothing and skips both the upload warning
    // and the parse block while the backend still admits it as visual.
    expect(getFileTypeByExtension(getExtension('clip.mov '))).toBe(
      FileType.Video,
    );
    expect(switchTarget('clip.mov ')).toBe('picture');
    // The trim is shared: other consumers resolve trailing-space names too.
    expect(getExtension('report.pdf ')).toBe('pdf');
  });

  it('returns undefined for unknown extensions', () => {
    expect(getFileTypeByExtension('xyz')).toBeUndefined();
    expect(getFileTypeByExtension('')).toBeUndefined();
  });
});

// Saved config shaped like a pipeline that declares pdf/image plus audio
// (ASR model missing) and video. Video needs no model: parsing it is
// unsupported regardless of the configured vision enhancement.
const SavedConfig: SavedParserConfig = {
  setups: [
    { fileFormat: FileType.PDF, parse_method: 'DeepDOC' },
    { fileFormat: FileType.Image, ocr_enabled: true },
    { fileFormat: FileType.Email, fields: ['subject', 'body'] },
    { fileFormat: FileType.Audio, vlm: { llm_id: '' } },
    { fileFormat: FileType.Video },
  ],
};

describe('findParserGap', () => {
  it('flags a file type missing from the saved setups as unsupported', () => {
    expect(findParserGap(FileType.Docx, SavedConfig)).toEqual({
      reason: ParserGapReason.UnsupportedType,
      fileType: FileType.Docx,
    });
  });

  it('flags audio/video without a declared family as unsupported, not missing-model', () => {
    const config: SavedParserConfig = {
      setups: [{ fileFormat: FileType.PDF }],
    };
    expect(findParserGap(FileType.Audio, config)).toEqual({
      reason: ParserGapReason.UnsupportedType,
      fileType: FileType.Audio,
    });
    expect(findParserGap(FileType.Video, config)).toEqual({
      reason: ParserGapReason.UnsupportedType,
      fileType: FileType.Video,
    });
  });

  it('flags declared audio without a model as a missing asr model', () => {
    expect(findParserGap(FileType.Audio, SavedConfig)).toEqual({
      reason: ParserGapReason.MissingModel,
      fileType: FileType.Audio,
      modelKind: ParserModelKind.Asr,
    });
  });

  it('flags video as unsupported even when a vision model is configured', () => {
    // Video parsing is not implemented yet: a declared family with a vision
    // model still stays unsupported so upload warns and parse-click blocks.
    expect(findParserGap(FileType.Video, SavedConfig)).toEqual({
      reason: ParserGapReason.UnsupportedType,
      fileType: FileType.Video,
    });
  });

  it('flags declared video as unsupported regardless of its setup', () => {
    const withModel: SavedParserConfig = {
      setups: [
        { fileFormat: FileType.Video, vlm: { llm_id: 'gpt-4o@OpenAI' } },
      ],
    };
    const withoutModel: SavedParserConfig = {
      setups: [{ fileFormat: FileType.Video, vlm: { llm_id: '' } }],
    };
    const expected = {
      reason: ParserGapReason.UnsupportedType,
      fileType: FileType.Video,
    };
    expect(findParserGap(FileType.Video, withModel)).toEqual(expected);
    expect(findParserGap(FileType.Video, withoutModel)).toEqual(expected);
  });

  it('passes image/email with a general-like setup — no switch needed', () => {
    expect(findParserGap(FileType.Image, SavedConfig)).toBeNull();
    expect(findParserGap(FileType.Email, SavedConfig)).toBeNull();
  });

  it('passes image with an ocr-only setup — no model required', () => {
    expect(findParserGap(FileType.Image, SavedConfig)).toBeNull();
  });

  it('passes supported non-media types and unknown file types', () => {
    expect(findParserGap(FileType.PDF, SavedConfig)).toBeNull();
    expect(findParserGap(undefined, SavedConfig)).toBeNull();
  });
});

describe('findFilesParserGaps', () => {
  it('returns a gap per failing file, mixing both reasons', () => {
    const gaps = findFilesParserGaps(
      [
        'song.mp3',
        'notes.docx',
        'photo.jpg',
        'movie.mp4',
        'deck.pptx',
        'mail.eml',
      ],
      SavedConfig,
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
      {
        name: 'movie.mp4',
        reason: ParserGapReason.UnsupportedType,
        fileType: FileType.Video,
      },
      {
        name: 'deck.pptx',
        reason: ParserGapReason.UnsupportedType,
        fileType: FileType.PowerPoint,
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

describe('builtinParserSwitchTarget', () => {
  it('maps owned file types to their built-in pipelines', () => {
    // Images switch to picture; video uploads are accepted into the visual
    // bucket and switched to picture as well, but parsing stays blocked
    // because no parser implements video analysis.
    expect(switchTarget('photo.JPG')).toBe('picture');
    expect(switchTarget('movie.mp4')).toBe('picture');
    expect(switchTarget('clip.mov')).toBe('picture');
    expect(switchTarget('clip.webm')).toBe('picture');
    expect(switchTarget('clip.rmvb')).toBe('picture');
    expect(switchTarget('clip.asf')).toBe('picture');
    expect(switchTarget('clip.mpa')).toBe('picture');
    expect(switchTarget('song.mp3')).toBe('audio');
    expect(switchTarget('song.opus')).toBe('audio');
    expect(switchTarget('deck.ppt')).toBe('presentation');
    expect(switchTarget('deck.pptx')).toBe('presentation');
    expect(switchTarget('deck.pages')).toBe('presentation');
    expect(switchTarget('mail.msg')).toBe('email');
    expect(switchTarget('mail.eml')).toBe('email');
  });

  it('falls back to the general pipeline for the families it declares', () => {
    // The backend reseeds these with the catch-all general pipeline when the
    // dataset selection does not declare the family (the upload-gap case).
    expect(switchTarget('laws.txt')).toBe('general');
    expect(switchTarget('laws.pdf')).toBe('general');
    expect(switchTarget('book.docx')).toBe('general');
    expect(switchTarget('sheet.xlsx')).toBe('general');
    expect(switchTarget('notes.md')).toBe('general');
    expect(switchTarget('page.html')).toBe('general');
  });

  it('keeps the dataset selection for types no pipeline owns', () => {
    // Frontend-known audio suffixes the backend rejects at upload keep the
    // dataset selection too — the switch map mirrors the backend exactly.
    expect(switchTarget('song.wma')).toBeNull();
  });
});

describe('attachAutoSwitchTargets', () => {
  it('annotates unsupported gaps with the target and other gaps with null', () => {
    const gaps = attachAutoSwitchTargets([
      {
        name: 'photo.jpg',
        reason: ParserGapReason.UnsupportedType,
        fileType: FileType.Image,
      },
      {
        name: 'song.mp3',
        reason: ParserGapReason.MissingModel,
        fileType: FileType.Audio,
        modelKind: ParserModelKind.Asr,
      },
      {
        name: 'notes.txt',
        reason: ParserGapReason.UnsupportedType,
        fileType: FileType.Code,
      },
    ]);
    expect(gaps.map((x) => x.autoSwitchTarget)).toEqual([
      'picture',
      null,
      'general',
    ]);
  });
});

describe('pickBySwitchFate', () => {
  const Options = {
    missingModel: 'missing',
    autoSwitch: 'auto',
    manualSwitch: 'manual',
    mixedSwitch: 'mixed',
  } as const;

  const unsupportedGap = (
    autoSwitchTarget: UploadFileParserGap['autoSwitchTarget'],
  ): UploadFileParserGap => ({
    name: 'f',
    reason: ParserGapReason.UnsupportedType,
    fileType: FileType.PDF,
    autoSwitchTarget,
  });

  const missingModelGap: UploadFileParserGap = {
    name: 'song.mp3',
    reason: ParserGapReason.MissingModel,
    fileType: FileType.Audio,
    modelKind: ParserModelKind.Asr,
    autoSwitchTarget: null,
  };

  it('picks the missing-model hint when nothing is unsupported', () => {
    expect(pickBySwitchFate([missingModelGap], Options)).toBe('missing');
  });

  it('picks the auto-switch hint only when every unsupported file switches', () => {
    expect(
      pickBySwitchFate(
        [unsupportedGap('picture'), unsupportedGap('email')],
        Options,
      ),
    ).toBe('auto');
  });

  it('picks the manual hint when no unsupported file switches', () => {
    expect(pickBySwitchFate([unsupportedGap(null)], Options)).toBe('manual');
  });

  it('picks the mixed hint when only some unsupported files switch', () => {
    expect(
      pickBySwitchFate(
        [unsupportedGap('picture'), unsupportedGap(null)],
        Options,
      ),
    ).toBe('mixed');
    // Missing-model gaps join the list without diluting the composition.
    expect(
      pickBySwitchFate([unsupportedGap(null), missingModelGap], Options),
    ).toBe('manual');
  });

  it('does not count video as switchable even though it targets picture', () => {
    // Video uploads switch to picture but stay unparsable, so a batch of a
    // video plus a non-switchable file must read as manual, not mixed; a
    // video plus an auto-switchable file reads as mixed, not auto.
    const videoGap: UploadFileParserGap = {
      name: 'clip.mov',
      reason: ParserGapReason.UnsupportedType,
      fileType: FileType.Video,
      autoSwitchTarget: 'picture',
    };
    expect(pickBySwitchFate([videoGap], Options)).toBe('manual');
    expect(
      pickBySwitchFate([videoGap, unsupportedGap('general')], Options),
    ).toBe('mixed');
  });
});

describe('getSavedParserConfig', () => {
  it('returns null without a saved parser config', () => {
    expect(getSavedParserConfig(null)).toBeNull();
    expect(
      getSavedParserConfig({
        parser_config: {},
      }),
    ).toBeNull();
  });

  it('returns null for legacy flat-shape configs without a Parser entry', () => {
    const knowledgeBase = {
      parser_config: { chunk_token_num: 128, delimiter: '\n' },
    };
    expect(getSavedParserConfig(knowledgeBase)).toBeNull();
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
    const config = getSavedParserConfig(knowledgeBase);
    expect(config?.setups.map((x) => x.fileFormat).sort()).toEqual([
      'image',
      'pdf',
    ]);
  });

  it('reads only the saved Parser setups from the Parser entry', () => {
    const knowledgeBase = {
      parser_config: {
        'Parser:1': {
          enable_vision_enhancement: true,
          vlm: { llm_id: 'glm-4v@ZHIPU-AI', system_prompt: 'describe' },
          video: {},
        },
      },
    };
    expect(getSavedParserConfig(knowledgeBase)).toEqual({
      setups: [{ fileFormat: FileType.Video }],
    });
  });

  it('keeps family-local settings for configs saved before the vision move', () => {
    const knowledgeBase = {
      parser_config: {
        'Parser:1': {
          video: { vlm: { llm_id: 'gpt-4o@OpenAI' } },
        },
      },
    };
    const config = getSavedParserConfig(knowledgeBase);
    expect(config?.setups).toEqual([
      { fileFormat: FileType.Video, vlm: { llm_id: 'gpt-4o@OpenAI' } },
    ]);
  });

  it('ignores non-parser operator entries', () => {
    const knowledgeBase = {
      parser_config: {
        'TokenChunker:1': { chunk_token_size: 128 },
      },
    };
    expect(getSavedParserConfig(knowledgeBase)).toBeNull();
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

  it('falls back to the dataset-level config for rows without a Parser entry', () => {
    const legacyRow = { name: 'song.mp3', parser_config: {} };
    expect(findDocumentsParserGaps([legacyRow], SavedConfig)).toEqual([
      {
        name: 'song.mp3',
        reason: ParserGapReason.MissingModel,
        fileType: FileType.Audio,
        modelKind: ParserModelKind.Asr,
      },
    ]);
    // No fallback either: nothing can be determined for other types, so
    // nothing is flagged — except video, which is never parsable.
    expect(findDocumentsParserGaps([legacyRow], null)).toEqual([]);
    expect(
      findDocumentsParserGaps([{ name: 'clip.mov', parser_config: {} }], null),
    ).toEqual([
      {
        name: 'clip.mov',
        reason: ParserGapReason.UnsupportedType,
        fileType: FileType.Video,
      },
    ]);
  });

  it('evaluates each document against its own config in a mixed batch', () => {
    const inheritedRow = { name: 'photo.jpg', parser_config: undefined };
    const gaps = findDocumentsParserGaps(
      [OverriddenDoc, inheritedRow],
      // Dataset-level config declares only audio: the overridden pdf passes
      // while the inherited jpg is flagged.
      {
        setups: [
          { fileFormat: FileType.Audio, vlm: { llm_id: 'whisper@OpenAI' } },
        ],
      },
    );
    expect(gaps).toEqual([
      {
        name: 'photo.jpg',
        reason: ParserGapReason.UnsupportedType,
        fileType: FileType.Image,
      },
    ]);
  });

  it('reads the document row vision globals for video', () => {
    const videoDoc = {
      name: 'movie.mp4',
      parser_config: {
        'Parser:1': {
          enable_vision_enhancement: true,
          vlm: { llm_id: 'glm-4v@ZHIPU-AI' },
          video: {},
        },
      },
    };
    // Video stays unsupported even with a vision model configured.
    expect(findDocumentsParserGaps([videoDoc], null)).toEqual([
      {
        name: 'movie.mp4',
        reason: ParserGapReason.UnsupportedType,
        fileType: FileType.Video,
      },
    ]);
  });
});
