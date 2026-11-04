import { ParseType } from '@/constants/knowledge';
import { formSchema } from './form-schema';

// A dataset created through POST /api/v1/datasets with
// `chunk_method: "naive", parser_config: {"chunk_token_num": 256}` comes back
// from GET /api/v1/datasets/<id> with the table-only settings it never uses
// stored as explicit nulls (issue #20893). The settings form uses this schema
// via zodResolver, so `.optional()` rejected the nulls and Save logged
// "Validation errors" without sending any request.
const apiCreatedNaiveParserConfig = {
  layout_recognize: 'DeepDOC',
  chunk_token_num: 256,
  delimiter: '\\n',
  enable_children: true,
  children_delimiter: '\n',
  html4excel: false,
  table_column_mode: null,
  table_column_roles: null,
  table_column_names: null,
};

function datasetFormValues(parserConfig: Record<string, unknown>) {
  return {
    parse_type: ParseType.BuiltIn,
    name: 'api-created',
    description: '',
    permission: 'fit',
    language: 'English',
    avatar: null,
    embedding_model: 'Qwen/Qwen3-8B@SILICONFLOW',
    chunk_method: 'naive',
    pagerank: 0,
    connectors: [],
    parser_config: parserConfig,
  } as never;
}

describe('dataset configuration form schema', () => {
  it('accepts null table_column_* values returned by the API', () => {
    const result = formSchema.safeParse(
      datasetFormValues(apiCreatedNaiveParserConfig),
    );

    expect(result.success).toBe(true);
    expect(
      result.success && result.data.parser_config?.table_column_mode,
    ).toBeNull();
    expect(
      result.success && result.data.parser_config?.table_column_roles,
    ).toBeNull();
    expect(
      result.success && result.data.parser_config?.table_column_names,
    ).toBeNull();
  });

  it('still maps the legacy vectorize role to indexing', () => {
    const result = formSchema.safeParse(
      datasetFormValues({
        ...apiCreatedNaiveParserConfig,
        table_column_mode: 'manual',
        table_column_roles: { col_a: 'vectorize', col_b: 'metadata' },
        table_column_names: ['col_a', 'col_b'],
      }),
    );

    expect(result.success).toBe(true);
    expect(
      result.success && result.data.parser_config?.table_column_roles,
    ).toEqual({ col_a: 'indexing', col_b: 'metadata' });
  });

  it('rejects an unknown table_column_mode', () => {
    const result = formSchema.safeParse(
      datasetFormValues({
        ...apiCreatedNaiveParserConfig,
        table_column_mode: 'bogus',
      }),
    );

    expect(result.success).toBe(false);
  });

  it('still rejects table_column_names of the wrong type', () => {
    const result = formSchema.safeParse(
      datasetFormValues({
        ...apiCreatedNaiveParserConfig,
        table_column_names: 'col_a',
      }),
    );

    expect(result.success).toBe(false);
  });
});
