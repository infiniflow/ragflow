import { FileType } from '@/constants/file';
import { ModelTypeToField } from '@/constants/llm';
import { initialParserValues } from '../../constant/pipeline';
import {
  buildInitialParserSetup,
  buildInitialParserValues,
  normalizeParserFormValues,
} from './utils';

describe('parser-form utils', () => {
  describe('buildInitialParserSetup', () => {
    it('returns a deep copy of the default setup', () => {
      const setup = buildInitialParserSetup(FileType.PDF, {});
      expect(setup?.fileFormat).toBe(FileType.PDF);
      // Mutating the copy must not touch the shared defaults
      (setup as any).pages[0].from = 5;
      expect(buildInitialParserSetup(FileType.PDF, {})?.pages?.[0].from).toBe(
        1,
      );
    });

    it('prefills the tenant default model for audio only', () => {
      const dictionary = {
        [ModelTypeToField.vision]: 'gpt-4o@OpenAI',
        [ModelTypeToField.asr]: 'whisper@OpenAI',
      };
      // The vision model is a global parser option, so video no longer gets
      // a per-setup prefill.
      expect(buildInitialParserSetup(FileType.Video, dictionary)?.vlm).toBe(
        undefined,
      );
      expect(buildInitialParserSetup(FileType.Audio, dictionary)?.vlm).toEqual({
        llm_id: 'whisper@OpenAI',
      });
    });

    it('leaves other file types untouched even with defaults present', () => {
      const dictionary = {
        [ModelTypeToField.vision]: 'gpt-4o@OpenAI',
        [ModelTypeToField.asr]: 'whisper@OpenAI',
      };
      expect(buildInitialParserSetup(FileType.PDF, dictionary)?.vlm).toBe(
        undefined,
      );
    });

    it('keeps the empty model id when no tenant default exists', () => {
      expect(buildInitialParserSetup(FileType.Audio, {})?.vlm).toEqual({
        llm_id: '',
      });
    });

    it('returns undefined for a file type without a default setup', () => {
      expect(buildInitialParserSetup('nope' as FileType, {})).toBeUndefined();
    });
  });

  describe('buildInitialParserValues', () => {
    it('prefills the global vision model and keeps the enhancement off', () => {
      const values = buildInitialParserValues({
        [ModelTypeToField.vision]: 'gpt-4o@OpenAI',
        [ModelTypeToField.asr]: 'whisper@OpenAI',
      });

      expect(values.vlm).toEqual({ llm_id: 'gpt-4o@OpenAI' });
      expect(values.enable_vision_enhancement).toBe(false);

      const byFileType = new Map(
        values.setups.map((setup: any) => [setup.fileFormat, setup]),
      );
      expect(byFileType.get(FileType.Video)?.vlm).toBe(undefined);
      expect(byFileType.get(FileType.Audio)?.vlm).toEqual({
        llm_id: 'whisper@OpenAI',
      });
      expect(byFileType.get(FileType.PDF)?.vlm).toBe(undefined);
    });

    it('keeps every default file type and does not mutate the defaults', () => {
      const values = buildInitialParserValues({});
      expect(values.setups).toHaveLength(initialParserValues.setups.length);
      expect(initialParserValues.setups).toEqual(
        buildInitialParserValues({}).setups,
      );
    });
  });

  describe('normalizeParserFormValues', () => {
    it('lifts legacy per-setup vision values onto the top level', () => {
      const values = normalizeParserFormValues({
        setups: [
          {
            fileFormat: FileType.PDF,
            flatten_media_to_text: false,
            vlm: { llm_id: 'gpt-4o@OpenAI' },
          },
          { fileFormat: FileType.Audio, vlm: { llm_id: 'whisper@OpenAI' } },
        ],
      });

      expect(values.enable_vision_enhancement).toBe(true);
      expect(values.vlm).toEqual({ llm_id: 'gpt-4o@OpenAI' });
      expect(values.setups[0]).toEqual({ fileFormat: FileType.PDF });
      // Audio keeps its per-setup ASR model.
      expect(values.setups[1]).toEqual({
        fileFormat: FileType.Audio,
        vlm: { llm_id: 'whisper@OpenAI' },
      });
    });

    it('turns the enhancement off when no setup used the vision model', () => {
      const values = normalizeParserFormValues({
        setups: [
          { fileFormat: FileType.PDF, flatten_media_to_text: true },
          { fileFormat: FileType.Docx, flatten_media_to_text: true },
        ],
      });

      expect(values.enable_vision_enhancement).toBe(false);
      expect(values.vlm).toEqual({ llm_id: '' });
    });

    it('treats a non-empty per-setup model as enabled enhancement', () => {
      const values = normalizeParserFormValues({
        setups: [
          { fileFormat: FileType.Video, vlm: { llm_id: 'gpt-4o@OpenAI' } },
        ],
      });

      expect(values.enable_vision_enhancement).toBe(true);
      expect(values.vlm).toEqual({ llm_id: 'gpt-4o@OpenAI' });
      expect(values.setups[0]).toEqual({ fileFormat: FileType.Video });
    });

    it('is idempotent on already-normalized values', () => {
      const values = {
        vlm: { llm_id: 'gpt-4o@OpenAI' },
        enable_vision_enhancement: true,
        setups: [
          {
            fileFormat: FileType.PDF,
            flatten_media_to_text: true,
            vlm: { llm_id: 'stale@Model' },
          },
          { fileFormat: FileType.Audio, vlm: { llm_id: 'whisper@OpenAI' } },
        ],
      };

      const normalized = normalizeParserFormValues(values);
      expect(normalized).toEqual({
        vlm: { llm_id: 'gpt-4o@OpenAI' },
        enable_vision_enhancement: true,
        setups: [
          { fileFormat: FileType.PDF },
          { fileFormat: FileType.Audio, vlm: { llm_id: 'whisper@OpenAI' } },
        ],
      });
      expect(normalizeParserFormValues(normalized)).toEqual(normalized);
    });

    it('keeps an explicitly cleared top-level model', () => {
      const values = normalizeParserFormValues({
        vlm: { llm_id: '' },
        enable_vision_enhancement: true,
        setups: [{ fileFormat: FileType.PDF, vlm: { llm_id: 'stale@Model' } }],
      });

      expect(values.vlm).toEqual({ llm_id: '' });
    });
  });
});
