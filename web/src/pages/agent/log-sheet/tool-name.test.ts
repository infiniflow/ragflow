jest.mock('@/constants/agent', () => ({
  Operator: {
    QueritContents: 'QueritContents',
    QueritSearch: 'QueritSearch',
    Search1APICrawl: 'Search1APICrawl',
    Search1APISearch: 'Search1APISearch',
  },
}));

import { Operator } from '@/constants/agent';
import { getToolOperatorName } from './tool-name';

describe('getToolOperatorName', () => {
  it.each(['QueritSearch', 'querit_search'])(
    'maps the Querit timeline name %p to its operator',
    (toolName) => {
      expect(getToolOperatorName(toolName)).toBe(Operator.QueritSearch);
    },
  );

  it.each(['QueritContents', 'querit_contents'])(
    'maps the Querit Contents timeline name %p to its operator',
    (toolName) => {
      expect(getToolOperatorName(toolName)).toBe(Operator.QueritContents);
    },
  );

  it.each(['Search1APISearch', 'search1api_search'])(
    'maps the Search1API timeline name %p to its operator',
    (toolName) => {
      expect(getToolOperatorName(toolName)).toBe(Operator.Search1APISearch);
    },
  );

  it.each(['Search1APICrawl', 'search1api_crawl'])(
    'maps the Search1API crawl timeline name %p to its operator',
    (toolName) => {
      expect(getToolOperatorName(toolName)).toBe(Operator.Search1APICrawl);
    },
  );

  it.each([undefined, null, ''])(
    'returns an empty name for the missing value %p',
    (toolName) => {
      expect(getToolOperatorName(toolName)).toBe('');
    },
  );
});
