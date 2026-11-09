import { renderHook } from '@testing-library/react';
import { Operator } from '../constant';
import { useInitializeOperatorParams } from './use-add-node';
import { useAgentToolInitialValues } from './use-agent-tool-initial-values';
import { getQueritAgentInitialValues } from './querit-agent-initial-values';

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
