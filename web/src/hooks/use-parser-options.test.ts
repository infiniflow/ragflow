import { renderHook } from '@testing-library/react';
import { AgentListItemType } from '@/interfaces/database/agent';
import {
  buildParserOptionValue,
  parseParserOptionValue,
  ParserOptionKind,
  useParserOptions,
} from '@/hooks/use-parser-options';

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
    expect(buildParserOptionValue(ParserOptionKind.BuiltIn, 'general')).toBe(
      'builtin:general',
    );
    expect(buildParserOptionValue(ParserOptionKind.Pipeline, 'abc123')).toBe(
      'pipeline:abc123',
    );
  });

  it('round-trips builtin and pipeline values', () => {
    expect(parseParserOptionValue('builtin:general')).toEqual({
      kind: ParserOptionKind.BuiltIn,
      rawId: 'general',
    });
    expect(parseParserOptionValue('pipeline:abc123')).toEqual({
      kind: ParserOptionKind.Pipeline,
      rawId: 'abc123',
    });
  });

  it('keeps builtin and pipeline ids with the same bare value distinct', () => {
    // A short builtin id and a pipeline id that happens to share the bare value
    // must not collide once prefixed.
    const builtin = buildParserOptionValue(ParserOptionKind.BuiltIn, 'general');
    const pipeline = buildParserOptionValue(
      ParserOptionKind.Pipeline,
      'general',
    );
    expect(builtin).not.toBe(pipeline);
    expect(parseParserOptionValue(builtin)?.kind).toBe(
      ParserOptionKind.BuiltIn,
    );
    expect(parseParserOptionValue(pipeline)?.kind).toBe(
      ParserOptionKind.Pipeline,
    );
  });

  it('handles ids that themselves contain a colon', () => {
    const value = buildParserOptionValue(ParserOptionKind.Pipeline, 'a:b:c');
    expect(value).toBe('pipeline:a:b:c');
    expect(parseParserOptionValue(value)).toEqual({
      kind: ParserOptionKind.Pipeline,
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
  it('puts pipeline options first, then builtin options with plain labels', () => {
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
      kind: ParserOptionKind.Pipeline,
    });
    expect(opts[1].value).toBe('pipeline:pipe-2');
    // Builtin appended below; the label stays plain and the "built in" tag is
    // rendered by the select component based on `kind`.
    expect(opts[2]).toEqual({
      value: 'builtin:general',
      label: 'General',
      kind: ParserOptionKind.BuiltIn,
    });
    expect(opts[3].label).toBe('Book');
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
      (o) => o.kind === ParserOptionKind.Pipeline,
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
