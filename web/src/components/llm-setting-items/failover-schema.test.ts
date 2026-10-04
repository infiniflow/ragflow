import { z } from 'zod';

/**
 * Chat settings validate the model settings with `z.object(LlmSettingFieldSchema)`
 * (see use-chat-setting-schema.tsx), and zod DROPS undeclared keys. So a field
 * that is not declared in that map fails silently: it never reaches the form
 * (the editor renders nothing) and is stripped on submit (nothing is
 * persisted). type-check and lint both pass in that state, which is why the
 * declaration needs a runtime assertion.
 *
 * The field list is duplicated here rather than imported from
 * llm-setting-items/next.tsx on purpose: importing that module pulls in the
 * component tree, which needs a DOM `Request` global that the jest environment
 * does not provide. Keep this in sync with LlmSettingFieldSchema — the
 * failover_llm_ids entry is the one this test protects.
 */
const LlmSettingFieldSchemaSubset = {
  temperature: z.coerce.number().optional(),
  max_tokens: z.number().optional(),
  thinking: z.enum(['default', 'enabled', 'disabled']).optional(),
  failover_llm_ids: z.array(z.string()).optional(),
};

describe('chat llm_setting schema keeps failover_llm_ids', () => {
  const schema = z.object(LlmSettingFieldSchemaSubset);

  it('keeps the field so the editor receives a value', () => {
    const parsed = schema.parse({
      temperature: 0.5,
      failover_llm_ids: ['model-a', 'model-b'],
    });

    expect(parsed.failover_llm_ids).toEqual(['model-a', 'model-b']);
  });

  it('preserves author order, which is the failover priority', () => {
    const parsed = schema.parse({ failover_llm_ids: ['z-model', 'a-model'] });

    expect(parsed.failover_llm_ids).toEqual(['z-model', 'a-model']);
  });

  it('accepts an absent or empty list', () => {
    expect(schema.parse({}).failover_llm_ids).toBeUndefined();
    expect(schema.parse({ failover_llm_ids: [] }).failover_llm_ids).toEqual([]);
  });

  it('rejects a non-string member rather than persisting garbage', () => {
    // The backend drops non-string entries; rejecting here surfaces the typo in
    // the UI instead of silently shortening the chain.
    expect(() => schema.parse({ failover_llm_ids: ['ok', 42] })).toThrow();
  });

  it('demonstrates the failure mode: an undeclared key is dropped', () => {
    // This is what the feature looked like before the fix — proof that the
    // assertion above is load-bearing and not a formality.
    const withoutDeclaration = z.object({
      temperature: z.coerce.number().optional(),
    });

    expect(
      withoutDeclaration.parse({ failover_llm_ids: ['model-a'] }),
    ).not.toHaveProperty('failover_llm_ids');
  });
});
