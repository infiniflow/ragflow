import { transformExtractorConfigToForm } from '@/utils/pipeline-operator';
import { transformExtractorParams } from '../../utils';

let mockIsGoBackend = true;
jest.mock('@/utils/backend-runtime', () => ({
  isGoBackend: () => mockIsGoBackend,
  getBackendLanguage: () => (mockIsGoBackend ? 'go' : 'python'),
}));

describe('Extractor parameter transformations & precedence', () => {
  beforeEach(() => {
    mockIsGoBackend = true;
  });

  describe('transformExtractorParams', () => {
    it('synchronizes nested modular configs to flat fields and preserves nested objects', () => {
      const input: any = {
        summary: {
          enabled: true,
          system_prompt: 'Custom summary prompt',
        },
        metadata_config: {
          enabled: true,
          metadata: [{ key: 'category', type: 'string' }],
          built_in_metadata: [{ key: 'update_time', type: 'time' }],
        },
        keywords: {
          top_n: 5,
          system_prompt: 'KW prompt',
        },
        questions: {
          top_n: 3,
          system_prompt: 'Q prompt',
        },
        tags: {
          top_n: 2,
          tag_file_id: 'tag-123',
        },
        llm_id: 'gpt-4',
      };

      const result = transformExtractorParams(input);

      expect(result.enable_summary).toBe(1);
      expect(result.summary).toEqual({
        enabled: true,
        system_prompt: 'Custom summary prompt',
      });
      expect(result.enable_metadata).toBe(1);
      expect(result.metadata_config.enabled).toBe(true);
      expect(result.metadata_config.metadata).toHaveLength(1);
      expect(result.auto_keywords).toBe(5);
      expect(result.auto_questions).toBe(3);
      expect(result.auto_tags).toBe(2);
      expect(result.tag_file_id).toBe('tag-123');
    });

    it('gives nested modular enabled: false precedence over legacy flat enable_*: 1', () => {
      const input: any = {
        summary: {
          enabled: false,
          system_prompt: '',
        },
        enable_summary: 1,
        metadata_config: {
          enabled: false,
          metadata: [],
          built_in_metadata: [],
        },
        enable_metadata: 1,
      };

      const result = transformExtractorParams(input);

      expect(result.summary.enabled).toBe(false);
      expect(result.enable_summary).toBe(0);
      expect(result.metadata_config.enabled).toBe(false);
      expect(result.enable_metadata).toBe(0);
    });

    it('preserves custom field_name when summary is disabled', () => {
      const input: any = {
        summary: {
          enabled: false,
          system_prompt: '',
        },
        field_name: 'custom_chunk_field',
      };

      const result = transformExtractorParams(input);
      expect(result.field_name).toBe('custom_chunk_field');
    });
  });

  describe('transformExtractorConfigToForm', () => {
    it('normalizes legacy flat API format into nested form schema', () => {
      const config = {
        enable_summary: 1,
        sys_prompt: 'Old summary prompt',
        enable_metadata: 1,
        metadata: [{ key: 'author', type: 'string' }],
        built_in_metadata: [{ key: 'file_name', type: 'string' }],
        auto_keywords: 4,
        auto_questions: 2,
        auto_tags: 1,
        tag_file_id: 'tag-file-1',
      };

      const result = transformExtractorConfigToForm(config);

      expect(result.summary).toEqual({
        enabled: true,
        system_prompt: 'Old summary prompt',
      });
      expect(result.metadata_config).toEqual({
        enabled: true,
        metadata: [{ key: 'author', type: 'string' }],
        built_in_metadata: [{ key: 'file_name', type: 'string' }],
      });
      expect(result.keywords.top_n).toBe(4);
      expect(result.questions.top_n).toBe(2);
      expect(result.tags.top_n).toBe(1);
    });
  });

  describe('python backend keeps the legacy flat shape', () => {
    beforeEach(() => {
      mockIsGoBackend = false;
    });

    it('transformExtractorParams only wraps prompts, without nested configs', () => {
      const input: any = {
        field_name: 'summary',
        sys_prompt: 'sys',
        prompts: 'user prompt',
        auto_keywords: 3,
        auto_questions: 2,
        auto_tags: 1,
        tag_file_id: 'tag-1',
        llm_id: 'gpt-4',
      };

      const result = transformExtractorParams(input);

      expect(result).toEqual({
        ...input,
        prompts: [{ content: 'user prompt', role: 'user' }],
      });
      expect(result).not.toHaveProperty('keywords');
      expect(result).not.toHaveProperty('questions');
      expect(result).not.toHaveProperty('tags');
      expect(result).not.toHaveProperty('summary');
      expect(result).not.toHaveProperty('metadata_config');
    });

    it('transformExtractorConfigToForm only unwraps prompts, without nested configs', () => {
      const config = {
        prompts: [{ content: 'user prompt', role: 'user' }],
        auto_keywords: 4,
        auto_tags: 1,
      };

      const result = transformExtractorConfigToForm(config);

      expect(result.prompts).toBe('user prompt');
      expect(result).not.toHaveProperty('keywords');
      expect(result).not.toHaveProperty('summary');
      expect(result).not.toHaveProperty('metadata_config');
      expect(result).not.toHaveProperty('enable_summary');
      expect(result).not.toHaveProperty('enable_metadata');
    });
  });
});
