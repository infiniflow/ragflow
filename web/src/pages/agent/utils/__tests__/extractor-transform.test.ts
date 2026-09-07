import { transformExtractorConfigToForm } from '@/utils/pipeline-operator';
import { transformExtractorParams } from '../../utils';

let mockIsGoBackend = true;
jest.mock('@/utils/backend-runtime', () => ({
  getBackendLanguage: () => (mockIsGoBackend ? 'go' : 'python'),
}));

describe('Extractor parameter transformations & precedence', () => {
  beforeEach(() => {
    mockIsGoBackend = true;
  });

  describe('transformExtractorParams', () => {
    it('emits only the LLM settings and the nested groups the Go extractor reads', () => {
      const input: any = {
        summary: {
          enabled: true,
          system_prompt: 'Custom summary prompt',
        },
        metadata: {
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
        temperature: 0.5,
        temperatureEnabled: true,
        // Fields that must not leak into the DSL params
        outputs: { chunks: { type: 'Array<Object>', value: [] } },
        prompts: 'user prompt',
        sys_prompt: 'legacy sys',
        field_name: 'summary',
        auto_keywords: 9,
        keywords_sys_prompt: 'legacy kw',
      };

      const result = transformExtractorParams(input);

      expect(result).toEqual({
        llm_id: 'gpt-4',
        temperature: 0.5,
        temperatureEnabled: true,
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
        summary: {
          enabled: true,
          system_prompt: 'Custom summary prompt',
        },
        metadata: {
          enabled: true,
          metadata: [{ key: 'category', type: 'string' }],
          built_in_metadata: [{ key: 'update_time', type: 'time' }],
        },
      });
    });

    it('maps legacy flat fields into the nested groups without re-emitting them', () => {
      const input: any = {
        auto_keywords: 4,
        keywords_sys_prompt: 'legacy kw prompt',
        auto_questions: 2,
        questions_sys_prompt: 'legacy q prompt',
        auto_tags: 1,
        tag_file_id: 'tag-legacy',
        enable_summary: 1,
        sys_prompt: 'legacy summary prompt',
      };

      const result = transformExtractorParams(input);

      expect(result).toEqual({
        keywords: { top_n: 4, system_prompt: 'legacy kw prompt' },
        questions: { top_n: 2, system_prompt: 'legacy q prompt' },
        tags: { top_n: 1, tag_file_id: 'tag-legacy' },
        summary: { enabled: true, system_prompt: 'legacy summary prompt' },
        metadata: { enabled: false, metadata: [], built_in_metadata: [] },
      });
    });

    it('accepts legacy flat metadata fields from unopened legacy nodes', () => {
      const input: any = {
        enable_metadata: 1,
        metadata: [{ key: 'author', type: 'string' }],
        built_in_metadata: [{ key: 'file_name', type: 'string' }],
      };

      const result = transformExtractorParams(input);

      expect(result).toEqual({
        keywords: { top_n: 0, system_prompt: '' },
        questions: { top_n: 0, system_prompt: '' },
        tags: { top_n: 0, tag_file_id: '' },
        summary: { enabled: false, system_prompt: '' },
        metadata: {
          enabled: true,
          metadata: [{ key: 'author', type: 'string' }],
          built_in_metadata: [{ key: 'file_name', type: 'string' }],
        },
      });
    });

    it('accepts the transitional metadata_config key', () => {
      const input: any = {
        metadata_config: {
          enabled: true,
          metadata: [{ key: 'category', type: 'string' }],
          built_in_metadata: [],
        },
      };

      const result = transformExtractorParams(input);

      expect(result.metadata).toEqual({
        enabled: true,
        metadata: [{ key: 'category', type: 'string' }],
        built_in_metadata: [],
      });
      expect(result).not.toHaveProperty('metadata_config');
    });

    it('gives nested modular enabled: false precedence over legacy flat enable_*: 1', () => {
      const input: any = {
        summary: {
          enabled: false,
          system_prompt: '',
        },
        enable_summary: 1,
        metadata: {
          enabled: false,
          metadata: [],
          built_in_metadata: [],
        },
        enable_metadata: 1,
      };

      const result = transformExtractorParams(input);

      expect(result.summary.enabled).toBe(false);
      expect(result.metadata.enabled).toBe(false);
      expect(result).not.toHaveProperty('enable_summary');
      expect(result).not.toHaveProperty('enable_metadata');
      expect(result).not.toHaveProperty('built_in_metadata');
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
      expect(result.metadata).toEqual({
        enabled: true,
        metadata: [{ key: 'author', type: 'string' }],
        built_in_metadata: [{ key: 'file_name', type: 'string' }],
      });
      expect(result).not.toHaveProperty('metadata_config');
      expect(result).not.toHaveProperty('enable_metadata');
      expect(result).not.toHaveProperty('built_in_metadata');
      expect(result.keywords.top_n).toBe(4);
      expect(result.questions.top_n).toBe(2);
      expect(result.tags.top_n).toBe(1);
    });

    it('passes through the metadata group shape unchanged', () => {
      const config = {
        metadata: {
          enabled: true,
          metadata: [{ key: 'category', type: 'string', enum: ['科技'] }],
          built_in_metadata: [{ key: 'doc_name', type: 'string' }],
        },
      };

      const result = transformExtractorConfigToForm(config);

      expect(result.metadata).toEqual({
        enabled: true,
        metadata: [{ key: 'category', type: 'string', enum: ['科技'] }],
        built_in_metadata: [{ key: 'doc_name', type: 'string' }],
      });
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
