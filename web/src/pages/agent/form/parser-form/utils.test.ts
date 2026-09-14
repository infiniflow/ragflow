import { ParseDocumentType } from '@/components/layout-recognize-form-field';
import { ModelTypeToField } from '@/constants/llm';
import {
  FileType,
  ImageParseMethod,
  initialParserValues,
} from '../../constant/pipeline';
import {
  buildInitialParserSetup,
  buildInitialParserValues,
  isStaticParseMethod,
} from './utils';

describe('parser-form utils', () => {
  describe('isStaticParseMethod', () => {
    it('recognizes the static parse methods', () => {
      expect(isStaticParseMethod(ImageParseMethod.OCR)).toBe(true);
      expect(isStaticParseMethod(ParseDocumentType.DeepDOC)).toBe(true);
      expect(isStaticParseMethod(ParseDocumentType.PlainText)).toBe(true);
      expect(isStaticParseMethod(ParseDocumentType.Docling)).toBe(true);
      expect(isStaticParseMethod(ParseDocumentType.OpenDataLoader)).toBe(true);
      expect(isStaticParseMethod(ParseDocumentType.TCADPParser)).toBe(true);
    });

    it('treats LLM model ids and empty values as non-static', () => {
      expect(isStaticParseMethod('gpt-4o@OpenAI')).toBe(false);
      expect(isStaticParseMethod('')).toBe(false);
      expect(isStaticParseMethod(undefined)).toBe(false);
      expect(isStaticParseMethod(null)).toBe(false);
    });
  });

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

    it('prefills the tenant default model for video and audio', () => {
      const dictionary = {
        [ModelTypeToField.vision]: 'gpt-4o@OpenAI',
        [ModelTypeToField.asr]: 'whisper@OpenAI',
      };
      expect(buildInitialParserSetup(FileType.Video, dictionary)?.vlm).toEqual({
        llm_id: 'gpt-4o@OpenAI',
      });
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
    it('prefills tenant default models across every default file type', () => {
      const values = buildInitialParserValues({
        [ModelTypeToField.vision]: 'gpt-4o@OpenAI',
        [ModelTypeToField.asr]: 'whisper@OpenAI',
      });

      const byFileType = new Map(
        values.setups.map((setup: any) => [setup.fileFormat, setup]),
      );
      expect(byFileType.get(FileType.Video)?.vlm).toEqual({
        llm_id: 'gpt-4o@OpenAI',
      });
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
});
