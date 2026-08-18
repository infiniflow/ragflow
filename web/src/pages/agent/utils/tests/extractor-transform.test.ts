import { transformExtractorConfigToForm } from '@/utils/pipeline-operator';
import { transformExtractorParams } from '../../utils';

describe('Extractor parameter transformations & precedence', () => {
  describe('transformExtractorParams', () => {
    it('transforms nested modular configs to clean API payload', () => {
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
      };

      const result = transformExtractorParams(input);

      expect(result.summary).toEqual({
        enabled: true,
        system_prompt: 'Custom summary prompt',
      });
      expect(result.metadata.enabled).toBe(true);
      expect(result.metadata.metadata).toHaveLength(1);
      expect(result.metadata.built_in_metadata).toHaveLength(1);
      expect(result.keywords).toEqual({
        top_n: 5,
        system_prompt: 'KW prompt',
      });
      expect(result.questions).toEqual({
        top_n: 3,
        system_prompt: 'Q prompt',
      });
      expect(result.tags).toEqual({
        top_n: 2,
        tag_file_id: 'tag-123',
      });
      expect(result.field_name).toBe('summary');
    });

    it('correctly handles disabled summary and metadata', () => {
      const input: any = {
        summary: {
          enabled: false,
          system_prompt: '',
        },
        metadata: {
          enabled: false,
          metadata: [],
          built_in_metadata: [],
        },
      };

      const result = transformExtractorParams(input);

      expect(result.summary.enabled).toBe(false);
      expect(result.metadata.enabled).toBe(false);
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

    it('omits obsolete flat fields from a legacy input payload', () => {
      const input: any = {
        enable_summary: 1,
        sys_prompt: 'Old summary prompt',
        enable_metadata: 1,
        metadata: [{ key: 'author', type: 'string' }],
        built_in_metadata: [{ key: 'file_name', type: 'string' }],
        auto_keywords: 4,
        keywords_sys_prompt: 'Old KW prompt',
        auto_questions: 2,
        questions_sys_prompt: 'Old Q prompt',
        auto_tags: 1,
        tag_file_id: 'tag-file-1',
        prompts: 'Old user prompt',
      };

      const result = transformExtractorParams(input);

      expect(result.enable_summary).toBeUndefined();
      expect(result.enable_metadata).toBeUndefined();
      expect(result.metadata_config).toBeUndefined();
      expect(result.sys_prompt).toBeUndefined();
      expect(result.prompts).toBeUndefined();
      expect(result.auto_keywords).toBeUndefined();
      expect(result.keywords_sys_prompt).toBeUndefined();
      expect(result.auto_questions).toBeUndefined();
      expect(result.questions_sys_prompt).toBeUndefined();
      expect(result.auto_tags).toBeUndefined();
      expect(result.tag_file_id).toBeUndefined();
      expect(result.summary).toEqual({
        enabled: true,
        system_prompt: 'Old summary prompt',
      });
      expect(result.metadata.enabled).toBe(true);
      expect(result.metadata.metadata).toHaveLength(1);
      expect(result.metadata.built_in_metadata).toHaveLength(1);
    });
  });

  describe('transformExtractorConfigToForm', () => {
    it('normalizes API format into nested form schema', () => {
      const config = {
        summary: {
          enabled: true,
          system_prompt: 'Standard summary prompt',
        },
        metadata: {
          enabled: true,
          metadata: [{ key: 'author', type: 'string' }],
          built_in_metadata: [{ key: 'file_name', type: 'string' }],
        },
        keywords: {
          top_n: 4,
          system_prompt: 'KW prompt',
        },
        questions: {
          top_n: 2,
          system_prompt: 'Q prompt',
        },
        tags: {
          top_n: 1,
          tag_file_id: 'tag-file-1',
        },
      };

      const result = transformExtractorConfigToForm(config);

      expect(result.summary).toEqual({
        enabled: true,
        system_prompt: 'Standard summary prompt',
      });
      expect(result.metadata).toEqual({
        enabled: true,
        metadata: [{ key: 'author', type: 'string' }],
        built_in_metadata: [{ key: 'file_name', type: 'string' }],
      });
      expect(result.keywords).toEqual({
        top_n: 4,
        system_prompt: 'KW prompt',
      });
      expect(result.questions).toEqual({
        top_n: 2,
        system_prompt: 'Q prompt',
      });
      expect(result.tags).toEqual({
        top_n: 1,
        tag_file_id: 'tag-file-1',
      });
    });

    it('normalizes legacy flat API format into nested form schema', () => {
      const legacyConfig = {
        enable_summary: 1,
        sys_prompt: 'Old summary prompt',
        enable_metadata: 1,
        metadata: [{ key: 'author', type: 'string' }],
        built_in_metadata: [{ key: 'file_name', type: 'string' }],
        auto_keywords: 4,
        keywords_sys_prompt: 'Old KW prompt',
        auto_questions: 2,
        questions_sys_prompt: 'Old Q prompt',
        auto_tags: 1,
        tag_file_id: 'tag-file-1',
      };

      const result = transformExtractorConfigToForm(legacyConfig);

      expect(result.summary).toEqual({
        enabled: true,
        system_prompt: 'Old summary prompt',
      });
      expect(result.metadata).toEqual({
        enabled: true,
        metadata: [{ key: 'author', type: 'string' }],
        built_in_metadata: [{ key: 'file_name', type: 'string' }],
      });
      expect(result.keywords).toEqual({
        top_n: 4,
        system_prompt: 'Old KW prompt',
      });
      expect(result.questions).toEqual({
        top_n: 2,
        system_prompt: 'Old Q prompt',
      });
      expect(result.tags).toEqual({
        top_n: 1,
        tag_file_id: 'tag-file-1',
      });
    });
  });
});
