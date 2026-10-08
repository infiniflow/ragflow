import { renderHook } from '@testing-library/react';
import { AgentListItemType } from '@/interfaces/database/agent';
import {
  buildParserOptionValue,
  parseParserOptionValue,
  useParserOptions,
} from '@/hooks/use-parser-options';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => (key === 'builtInSuffix' ? ' (built in)' : key),
  }),
}));

jest.mock('@/hooks/use-agent-request', () => ({
  useFetchBuiltinPipelines: jest.fn(),
  useFetchAgentList: jest.fn(),
}));

// eslint-disable-next-line @typescript-eslint/no-var-requires
const agentRequest = require('@/hooks/use-agent-request');

beforeEach(() => {
  agentRequest.useFetchBuiltinPipelines.mockReturnValue({
    options: [],
    loading: false,
  });
  agentRequest.useFetchAgentList.mockReturnValue({
    data: { canvas: [] },
    loading: false,
  });
});

describe('parser option value encoding', () => {
  it('prefixes builtin and pipeline ids distinctly', () => {
    expect(buildParserOptionValue('builtin', 'general')).toBe(
      'builtin:general',
    );
    expect(buildParserOptionValue('pipeline', 'abc123')).toBe(
      'pipeline:abc123',
    );
  });

  it('round-trips builtin and pipeline values', () => {
    expect(parseParserOptionValue('builtin:general')).toEqual({
      kind: 'builtin',
      rawId: 'general',
    });
    expect(parseParserOptionValue('pipeline:abc123')).toEqual({
      kind: 'pipeline',
      rawId: 'abc123',
    });
  });

  it('keeps builtin and pipeline ids with the same bare value distinct', () => {
    // A short builtin id and a pipeline id that happens to share the bare value
    // must not collide once prefixed.
    const builtin = buildParserOptionValue('builtin', 'general');
    const pipeline = buildParserOptionValue('pipeline', 'general');
    expect(builtin).not.toBe(pipeline);
    expect(parseParserOptionValue(builtin)?.kind).toBe('builtin');
    expect(parseParserOptionValue(pipeline)?.kind).toBe('pipeline');
  });

  it('handles ids that themselves contain a colon', () => {
    const value = buildParserOptionValue('pipeline', 'a:b:c');
    expect(value).toBe('pipeline:a:b:c');
    expect(parseParserOptionValue(value)).toEqual({
      kind: 'pipeline',
      rawId: 'a:b:c',
    });
  });

  it('returns null for empty or unprefixed values', () => {
    expect(parseParserOptionValue('')).toBeNull();
    expect(parseParserOptionValue(undefined)).toBeNull();
    expect(parseParserOptionValue('general')).toBeNull();
    expect(parseParserOptionValue('unknown:general')).toBeNull();
  });
});

describe('useParserOptions merged list', () => {
  it('puts pipeline options first, then builtin options with the suffix', () => {
    agentRequest.useFetchBuiltinPipelines.mockReturnValue({
      options: [
        { label: 'General', value: 'general' },
        { label: 'Book', value: 'book' },
      ],
      loading: false,
    });
    agentRequest.useFetchAgentList.mockReturnValue({
      data: {
        canvas: [
          { id: 'pipe-1', title: 'My Flow', type: 'dataflow' },
          { id: 'pipe-2', title: 'Other', type: 'dataflow' },
        ],
      },
      loading: false,
    });

    const { result } = renderHook(() => useParserOptions());
    const opts = result.current.options;

    expect(opts).toHaveLength(4);
    // Pipelines first, in original order.
    expect(opts[0]).toEqual({
      value: 'pipeline:pipe-1',
      label: 'My Flow',
      kind: 'pipeline',
    });
    expect(opts[1].value).toBe('pipeline:pipe-2');
    // Builtin appended below with the " (built in)" suffix.
    expect(opts[2]).toEqual({
      value: 'builtin:general',
      label: 'General (built in)',
      kind: 'builtin',
    });
    expect(opts[3].label).toBe('Book (built in)');
  });

  it('excludes compilation template groups from pipeline options', () => {
    agentRequest.useFetchAgentList.mockReturnValue({
      data: {
        canvas: [
          { id: 'pipe-1', title: 'Real', type: 'dataflow' },
          {
            id: 'grp-1',
            title: 'Group',
            type: AgentListItemType.CompilationTemplateGroup,
          },
        ],
      },
      loading: false,
    });

    const { result } = renderHook(() => useParserOptions());
    const pipelineOpts = result.current.options.filter(
      (o) => o.kind === 'pipeline',
    );
    expect(pipelineOpts).toHaveLength(1);
    expect(pipelineOpts[0].value).toBe('pipeline:pipe-1');
  });

  it('handles empty results from both sources', () => {
    const { result } = renderHook(() => useParserOptions());
    expect(result.current.options).toEqual([]);
    expect(result.current.loading).toBe(false);
  });

  it('surfaces loading while either source is fetching', () => {
    agentRequest.useFetchBuiltinPipelines.mockReturnValue({
      options: [],
      loading: true,
    });
    const { result } = renderHook(() => useParserOptions());
    expect(result.current.loading).toBe(true);
  });
});
