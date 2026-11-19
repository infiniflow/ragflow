import { ParseType } from '@/constants/knowledge';
import { buildFormSchema } from './form-schema';

// The real module drags the compilation-template graph (markdown editor →
// mermaid/lexical ESM) into jsdom; these tests never exercise parser_config
// validation.
jest.mock(
  '@/components/pipeline-operator-tabs/parser-config-validation',
  () => ({
    addParserConfigIssues: () => {},
  }),
);

const BaseValues = {
  name: 'kb',
  embedding_model: 'text-embedding',
  pagerank: 0,
};

function missingIssuePaths(result: {
  success: boolean;
  error?: { issues: { path: (string | number)[] }[] };
}) {
  return (result.error?.issues ?? []).map((issue) => issue.path.join('.'));
}

describe('buildFormSchema parser-option-missing rule', () => {
  it('rejects a missing pipeline selection on the pipeline_id field', () => {
    const schema = buildFormSchema(() => true);
    const result = schema.safeParse({
      ...BaseValues,
      parse_type: ParseType.Pipeline,
      pipeline_id: 'deleted-pipeline',
      pipeline_version_id: 'v1',
    });

    expect(result.success).toBe(false);
    expect(missingIssuePaths(result)).toContain('pipeline_id');
  });

  it('rejects a missing builtin selection on the parser_id field', () => {
    const schema = buildFormSchema(() => true);
    const result = schema.safeParse({
      ...BaseValues,
      parse_type: ParseType.BuiltIn,
      parser_id: 'deleted-parser',
    });

    expect(result.success).toBe(false);
    expect(missingIssuePaths(result)).toContain('parser_id');
  });

  it.each([undefined, '', '   '])(
    'rejects a dataflow selection without a concrete version (%p)',
    (versionId) => {
      const schema = buildFormSchema(() => false);
      const result = schema.safeParse({
        ...BaseValues,
        parse_type: ParseType.Pipeline,
        pipeline_id: 'pipeline',
        pipeline_version_id: versionId,
      });

      expect(result.success).toBe(false);
      expect(missingIssuePaths(result)).toEqual(['pipeline_id']);
    },
  );

  it('passes when the selected dataflow version still exists', () => {
    const schema = buildFormSchema(() => false);
    const result = schema.safeParse({
      ...BaseValues,
      parse_type: ParseType.Pipeline,
      pipeline_id: 'pipeline',
      pipeline_version_id: 'version-1',
    });

    expect(result.success).toBe(true);
  });

  it('passes when the selected builtin parser still exists', () => {
    const schema = buildFormSchema(() => false);
    const result = schema.safeParse({
      ...BaseValues,
      parse_type: ParseType.BuiltIn,
      parser_id: 'general',
    });

    expect(result.success).toBe(true);
  });
});
