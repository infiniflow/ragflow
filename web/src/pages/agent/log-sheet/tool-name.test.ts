jest.mock('@/constants/agent', () => ({
  Operator: {
    QueritSearch: 'QueritSearch',
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

  it.each([undefined, null, ''])(
    'returns an empty name for the missing value %p',
    (toolName) => {
      expect(getToolOperatorName(toolName)).toBe('');
    },
  );
});
