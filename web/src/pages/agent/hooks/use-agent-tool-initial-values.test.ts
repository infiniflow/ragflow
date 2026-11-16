import { renderHook } from '@testing-library/react';
import {
  initialAnySearchExtractValues,
  initialAnySearchSearchValues,
  Operator,
} from '../constant';
import { getQueritAgentInitialValues } from './querit-agent-initial-values';
import { useInitializeOperatorParams } from './use-add-node';
import { useAgentToolInitialValues } from './use-agent-tool-initial-values';

jest.mock('@/hooks/use-llm-request', () => ({
  useFetchDefaultModelDictionary: () => ({ llm_id: 'model-A' }),
}));

jest.mock('./use-add-node', () => ({
  useInitializeOperatorParams: jest.fn(),
}));

describe('getQueritAgentInitialValues', () => {
  it('creates Querit Agent params without runtime query or outputs', () => {
    expect(
      getQueritAgentInitialValues({
        api_key: '',
        query: 'sys.query',
        count: 10,
        chunks_per_doc: 3,
        site_include: [],
        site_exclude: [],
        time_range: '',
        country_include: [],
        language_include: [],
        outputs: {
          formalized_content: { value: '', type: 'string' },
          json: { value: {}, type: 'object' },
        },
      }),
    ).toEqual({
      api_key: '',
      count: 10,
      chunks_per_doc: 3,
      site_include: [],
      site_exclude: [],
      time_range: '',
      country_include: [],
      language_include: [],
    });
  });
});

describe('useAgentToolInitialValues', () => {
  beforeEach(() => {
    jest.mocked(useInitializeOperatorParams).mockReset();
    jest.mocked(useInitializeOperatorParams).mockReturnValue({
      initialFormValuesMap: {
        [Operator.AnySearchSearch]: { ...initialAnySearchSearchValues },
        [Operator.AnySearchExtract]: { ...initialAnySearchExtractValues },
      },
    } as any);
  });

  it('creates AnySearch Search tool params without runtime query or outputs', () => {
    const { result } = renderHook(() => useAgentToolInitialValues());

    expect(
      result.current.initializeAgentToolValues(Operator.AnySearchSearch),
    ).toEqual({ api_key: '', top_n: 10, zone: '', language: '' });
  });

  it('creates AnySearch Discover tool params with only the optional key', () => {
    const { result } = renderHook(() => useAgentToolInitialValues());

    expect(
      result.current.initializeAgentToolValues(Operator.AnySearchDiscover),
    ).toEqual({ api_key: '' });
  });

  it('creates AnySearch Extract tool params with only the optional key', () => {
    const { result } = renderHook(() => useAgentToolInitialValues());

    expect(
      result.current.initializeAgentToolValues(Operator.AnySearchExtract),
    ).toEqual({ api_key: '' });
  });

  it('does not reuse canvas defaults for a Tavily Search agent tool', () => {
    jest.mocked(useInitializeOperatorParams).mockReturnValue({
      initialFormValuesMap: {
        [Operator.TavilySearch]: {
          api_key: 'canvas-api-key',
          search_depth: 'advanced',
          max_results: 10,
        },
        [Operator.TavilyExtract]: {
          api_key: 'canvas-api-key',
          extract_depth: 'advanced',
        },
      },
    } as any);

    const { result } = renderHook(() => useAgentToolInitialValues());

    expect(
      result.current.initializeAgentToolValues(Operator.TavilySearch),
    ).toEqual({ api_key: '' });
  });
});
