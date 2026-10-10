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

      expect(values.vlm).toEqual({
        llm_id: 'gpt-4o@OpenAI',
        lang: '',
        system_prompt: '',
      });
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
      expect(values.vlm).toEqual({
        llm_id: 'gpt-4o@OpenAI',
        lang: '',
        system_prompt: '',
      });
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
      expect(values.vlm).toEqual({ llm_id: '', lang: '', system_prompt: '' });
    });

    it('treats a non-empty per-setup model as enabled enhancement', () => {
      const values = normalizeParserFormValues({
        setups: [
          { fileFormat: FileType.Video, vlm: { llm_id: 'gpt-4o@OpenAI' } },
        ],
      });

      expect(values.enable_vision_enhancement).toBe(true);
      expect(values.vlm).toEqual({
        llm_id: 'gpt-4o@OpenAI',
        lang: '',
        system_prompt: '',
      });
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
        vlm: { llm_id: 'gpt-4o@OpenAI', lang: '', system_prompt: '' },
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

      expect(values.vlm).toEqual({ llm_id: '', lang: '', system_prompt: '' });
    });

    it('migrates a legacy image ocr parse_method onto ocr_enabled', () => {
      const normalized = normalizeParserFormValues({
        setups: [{ fileFormat: FileType.Image, parse_method: 'ocr' }],
      });
      expect(normalized.setups[0]).toEqual({
        fileFormat: FileType.Image,
        ocr_enabled: true,
      });
      // An empty parse_method also selects OCR.
      expect(
        normalizeParserFormValues({
          setups: [{ fileFormat: FileType.Image, parse_method: '' }],
        }).setups[0],
      ).toEqual({ fileFormat: FileType.Image, ocr_enabled: true });
    });

    it('lifts a legacy image model reference onto the global vlm and turns OCR off', () => {
      const normalized = normalizeParserFormValues({
        setups: [
          { fileFormat: FileType.Image, parse_method: 'gpt-4o@OpenAI' },
          { fileFormat: FileType.PDF },
        ],
      });
      // The model is not lost: it becomes the shared vision model, overriding
      // the (empty) global value, and the switch shape drops parse_method.
      expect(normalized.vlm).toEqual({
        llm_id: 'gpt-4o@OpenAI',
        lang: '',
        system_prompt: '',
      });
      expect(normalized.setups[0]).toEqual({
        fileFormat: FileType.Image,
        ocr_enabled: false,
      });
    });

    it('lifts the image family language and prompt onto the global vlm', () => {
      const normalized = normalizeParserFormValues({
        setups: [
          {
            fileFormat: FileType.Image,
            ocr_enabled: true,
            lang: 'French',
            system_prompt: 'Describe the chart.',
          },
        ],
      });
      expect(normalized.vlm).toEqual({
        llm_id: '',
        lang: 'French',
        system_prompt: 'Describe the chart.',
      });
      // The image setup keeps only its own switch.
      expect(normalized.setups[0]).toEqual({
        fileFormat: FileType.Image,
        ocr_enabled: true,
      });
      expect(normalizeParserFormValues(normalized)).toEqual(normalized);
    });

    it('does not let a stale image language override a global choice', () => {
      const normalized = normalizeParserFormValues({
        vlm: { llm_id: '', lang: 'German', system_prompt: '' },
        setups: [
          { fileFormat: FileType.Image, lang: 'French', system_prompt: 'old' },
        ],
      });
      // An explicit global language wins; the prompt still lifts because no
      // global prompt was chosen.
      expect(normalized.vlm).toEqual({
        llm_id: '',
        lang: 'German',
        system_prompt: 'old',
      });
    });

    it('leaves an existing global model untouched when the image already uses the switch', () => {
      // Migration only fires for a legacy parse_method. A normalized image
      // setup (ocr_enabled present, no parse_method) never overrides a model
      // the user chose at the top level.
      const normalized = normalizeParserFormValues({
        vlm: { llm_id: 'qwen-vl@DashScope' },
        setups: [{ fileFormat: FileType.Image, ocr_enabled: false }],
      });
      expect(normalized.vlm).toEqual({
        llm_id: 'qwen-vl@DashScope',
        lang: '',
        system_prompt: '',
      });
      expect(normalized.setups[0]).toEqual({
        fileFormat: FileType.Image,
        ocr_enabled: false,
      });
    });

    it('is idempotent for the migrated image switch', () => {
      const once = normalizeParserFormValues({
        setups: [{ fileFormat: FileType.Image, parse_method: 'ocr' }],
      });
      expect(normalizeParserFormValues(once)).toEqual(once);
    });
  });
});
