import { renderHook } from '@testing-library/react';
import { useFetchSessionList } from '../use-chat-request';

const mockUseSessionList = jest.fn();
let mockSearchString = '';

jest.mock('react-router', () => ({
  ...jest.requireActual('react-router'),
  useParams: () => ({ id: 'chat-1' }),
}));

jest.mock('@/services/next-chat-service', () => ({
  __esModule: true,
  default: {},
}));

jest.mock('../use-session-list', () => ({
  useSessionList: (options: unknown) => mockUseSessionList(options),
}));

jest.mock('../logic-hooks/use-change-search', () => ({
  useHandleSearchStrChange: () => ({
    searchString: mockSearchString,
    handleInputChange: jest.fn(),
    setSearchString: jest.fn(),
  }),
}));

describe('useFetchSessionList search count', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockUseSessionList.mockReturnValue({
      data: [
        { id: 'session-1', name: 'Alpha' },
        { id: 'session-2', name: 'Beta' },
      ],
      total: 20,
    });
  });

  it.each([
    {
      label: 'empty',
      keyword: '',
      expectedNames: ['Alpha', 'Beta'],
      expectedTotal: 20,
    },
    {
      label: 'whitespace-only',
      keyword: '   ',
      expectedNames: ['Alpha', 'Beta'],
      expectedTotal: 20,
    },
    {
      label: 'matching',
      keyword: ' Alpha ',
      expectedNames: ['Alpha'],
      expectedTotal: 1,
    },
    {
      label: 'non-matching',
      keyword: 'missing',
      expectedNames: [],
      expectedTotal: 0,
    },
  ])(
    'returns the expected list and count for a $label keyword',
    ({ keyword, expectedNames, expectedTotal }) => {
      mockSearchString = keyword;
      const { result } = renderHook(() => useFetchSessionList());

      expect(result.current.data.map((session) => session.name)).toEqual(
        expectedNames,
      );
      expect(result.current.total).toBe(expectedTotal);
    },
  );
});
