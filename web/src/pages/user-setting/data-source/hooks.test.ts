import { act, renderHook } from '@testing-library/react';

jest.mock('react-router', () => ({
  useParams: jest.fn(() => ({})),
  useSearchParams: jest.fn(() => [new URLSearchParams(''), () => {}]),
}));

jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

jest.mock('@/services/data-source-service', () => ({
  __esModule: true,
  default: {
    dataSourceList: jest.fn(),
    dataSourceUpdate: jest.fn(),
  },
  dataSourceRebuild: jest.fn(),
  dataSourceUpdate: jest.fn(),
  deleteDataSource: jest.fn(),
  featchDataSourceDetail: jest.fn(),
  getDataSourceLogs: jest.fn(),
  testDataSource: jest.fn(),
}));

jest.mock('i18next', () => ({
  t: jest.fn((key: string) => key),
}));

jest.mock('@tanstack/react-query', () => ({
  useQuery: jest.fn(),
  useQueryClient: jest.fn(() => ({})),
}));

jest.mock('@/hooks/logic-hooks', () => ({
  useGetPaginationWithRouter: jest.fn(),
}));

jest.mock('@/hooks/common-hooks', () => ({
  useSetModalState: jest.fn(),
}));

jest.mock('./constant', () => ({
  DataSourceKey: {},
  useDataSourceInfo: jest.fn(() => ({})),
}));

import { testDataSource } from '@/services/data-source-service';
import { useTestDataSource } from './hooks';

const mockTest = jest.mocked(testDataSource);

const baseFields = [
  { name: 'id', label: 'Id', type: 'text', hidden: true },
  { name: 'name', label: 'Name', type: 'text', required: true },
  { name: 'source', label: 'Source', type: 'text', required: true },
];

describe('useTestDataSource', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('does not run Test Connection when the required name field is empty', async () => {
    const formRef = {
      current: {
        getFilteredValues: () => ({ id: '1', name: '', source: 'mysql' }),
        trigger: jest.fn().mockResolvedValue(false),
      },
    };
    const { result } = renderHook(() =>
      useTestDataSource(formRef as never, undefined, baseFields as never),
    );

    await act(async () => {
      await result.current.handleTest();
    });

    expect(formRef.current.trigger).toHaveBeenCalledWith(
      expect.arrayContaining(['name']),
    );
    expect(mockTest).not.toHaveBeenCalled();
  });

  it('runs Test Connection when the required fields are valid', async () => {
    mockTest.mockResolvedValue({ data: { code: 0 } } as never);
    const formRef = {
      current: {
        getFilteredValues: () => ({ id: '1', name: 'conn', source: 'mysql' }),
        trigger: jest.fn().mockResolvedValue(true),
      },
    };
    const { result } = renderHook(() =>
      useTestDataSource(formRef as never, undefined, baseFields as never),
    );

    await act(async () => {
      await result.current.handleTest();
    });

    expect(formRef.current.trigger).toHaveBeenCalledWith(
      expect.arrayContaining(['name']),
    );
    expect(mockTest).toHaveBeenCalledWith('1', {
      source: 'mysql',
      config: {},
    });
  });

});
