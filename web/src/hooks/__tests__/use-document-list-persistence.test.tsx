import { documentFilter, listDocument } from '@/services/knowledge-service';
import { useSelectDatasetFilters } from '@/pages/dataset/dataset/use-select-filters';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import React from 'react';
import {
  DocumentKeys,
  useFetchDocumentList,
  useGetDocumentFilter,
} from '../use-document-request';

let mockDatasetId = 'dataset-a';
const mockSetPagination = jest.fn();

jest.mock('@/services/knowledge-service', () => ({
  __esModule: true,
  default: {},
  listDocument: jest.fn(),
  documentFilter: jest.fn(),
}));
jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));
jest.mock('@/locales/config', () => ({
  __esModule: true,
  default: { t: (key: string) => key },
}));
jest.mock('@/hooks/common-hooks', () => ({
  useTranslate: () => ({ t: (key: string) => key }),
}));
jest.mock('@/hooks/use-user-setting-request', () => ({
  useSaveSetting: jest.fn(),
}));
jest.mock('@/hooks/route-hook', () => ({
  useGetKnowledgeSearchParams: () => ({ knowledgeId: '' }),
  useSetPaginationParams: () => ({
    setPaginationParams: mockSetPagination,
    page: 1,
    size: 30,
  }),
}));
jest.mock('react-router', () => ({ useParams: () => ({ id: mockDatasetId }) }));

type FilterValue = import('@/components/list-filter-bar/interface').FilterValue;
type PropsWithChildren = import('react').PropsWithChildren;
type InputChange = import('react').ChangeEvent<HTMLInputElement>;

const AppliedFilter: FilterValue = {
  type: ['pdf'],
  run: ['3'],
  metadata: { author: ['Ada'] },
};

function createWrapper(
  client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  }),
) {
  const wrapper = ({ children }: PropsWithChildren) =>
    React.createElement(QueryClientProvider, { client }, children);
  return wrapper;
}

function mountList() {
  return renderHook(() => useFetchDocumentList(false), {
    wrapper: createWrapper(),
  });
}

function search(
  result: { current: { handleInputChange: (event: InputChange) => void } },
  value: string,
) {
  act(() => {
    result.current.handleInputChange({
      target: { value },
    } as InputChange);
  });
}

beforeEach(() => {
  jest.clearAllMocks();
  sessionStorage.clear();
  localStorage.clear();
  localStorage.setItem(
    'userInfo',
    JSON.stringify({ email: 'ada@example.com' }),
  );
  mockDatasetId = 'dataset-a';
  jest.mocked(listDocument).mockResolvedValue({
    data: { code: 0, data: { docs: [], total: 0 } },
  } as never);
});

it('restores the search and applied filters in the first request after remounting', async () => {
  const first = mountList();
  search(first.result, 'design');
  act(() => first.result.current.handleFilterSubmit(AppliedFilter));
  first.unmount();
  jest.mocked(listDocument).mockClear();

  const restored = mountList();
  expect(restored.result.current.searchString).toBe('design');
  expect(restored.result.current.filterValue).toEqual(AppliedFilter);
  await waitFor(() => expect(listDocument).toHaveBeenCalled());
  expect(jest.mocked(listDocument).mock.calls[0]).toEqual([
    { id: 'dataset-a', keywords: 'design', page: 1, page_size: 30 },
    {
      suffix: ['pdf'],
      run_status: ['COMPLETED'],
      return_empty_metadata: false,
      metadata: { author: ['Ada'] },
    },
  ]);
});

it('keeps cleared search and filters cleared after remounting', () => {
  const first = mountList();
  search(first.result, 'design');
  act(() => first.result.current.handleFilterSubmit(AppliedFilter));
  search(first.result, '');
  act(() => first.result.current.handleFilterSubmit({}));
  first.unmount();

  const restored = mountList();
  expect(restored.result.current.searchString).toBe('');
  expect(restored.result.current.filterValue).toEqual({});
  expect(mockSetPagination).toHaveBeenCalledWith(1, undefined);
});

it('isolates state and requests by dataset when the route changes without unmounting', async () => {
  const list = mountList();
  search(list.result, 'design');
  act(() => list.result.current.handleFilterSubmit(AppliedFilter));
  await waitFor(() =>
    expect(listDocument).toHaveBeenLastCalledWith(
      expect.objectContaining({ keywords: 'design' }),
      expect.anything(),
    ),
  );

  jest.mocked(listDocument).mockClear();
  mockDatasetId = 'dataset-b';
  list.rerender();
  expect(list.result.current.searchString).toBe('');
  expect(list.result.current.filterValue).toEqual({});
  await waitFor(() => expect(listDocument).toHaveBeenCalled());
  expect(jest.mocked(listDocument).mock.calls[0][0]).toMatchObject({
    id: 'dataset-b',
    keywords: '',
  });

  jest.mocked(listDocument).mockClear();
  search(list.result, 'm');
  search(list.result, 'manual');
  expect(listDocument).not.toHaveBeenCalled();
  await waitFor(() =>
    expect(listDocument).toHaveBeenLastCalledWith(
      expect.objectContaining({ id: 'dataset-b', keywords: 'manual' }),
      expect.anything(),
    ),
  );
  mockDatasetId = 'dataset-a';
  list.rerender();
  expect(list.result.current.searchString).toBe('design');
  expect(list.result.current.filterValue).toEqual(AppliedFilter);
});

it('keeps restored filters while the filter catalog is loading, then prunes missing values', () => {
  const first = mountList();
  act(() => first.result.current.handleFilterSubmit(AppliedFilter));
  first.unmount();
  const restored = mountList();

  act(() =>
    restored.result.current.checkValue(
      [
        { field: 'type', label: 'Type', list: undefined! },
        { field: 'run', label: 'Status', list: [] },
      ],
      false,
    ),
  );
  expect(restored.result.current.filterValue).toEqual(AppliedFilter);

  act(() =>
    restored.result.current.checkValue(
      [
        { field: 'type', label: 'Type', list: [{ id: 'pdf', label: 'PDF' }] },
        {
          field: 'run',
          label: 'Status',
          list: [{ id: '3', label: 'Completed' }],
        },
        {
          field: 'metadata',
          label: 'Metadata',
          list: [
            {
              id: 'author',
              label: 'Author',
              list: [{ id: 'Grace', label: 'Grace' }],
            },
          ],
        },
      ],
      true,
    ),
  );
  expect(restored.result.current.filterValue).toEqual({
    ...AppliedFilter,
    metadata: { author: [] },
  });
});

it.each(['empty', 'error'])(
  'handles an %s filter catalog without losing loading state',
  async (outcome) => {
    const first = mountList();
    act(() => first.result.current.handleFilterSubmit(AppliedFilter));
    first.unmount();
    let complete!: () => void;
    jest.mocked(documentFilter).mockReturnValue(
      new Promise((resolve, reject) => {
        complete = () =>
          outcome === 'error'
            ? reject(new Error('Catalog unavailable'))
            : resolve({
                data: {
                  code: 0,
                  data: {
                    filter: { suffix: {}, ingestion_status: {}, metadata: {} },
                  },
                },
              });
      }) as never,
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const restored = renderHook(
      () => {
        const list = useFetchDocumentList(false);
        const { checkValue } = list;
        const { filters, filterLoaded } = useSelectDatasetFilters();
        React.useEffect(() => {
          checkValue(filters, filterLoaded);
        }, [checkValue, filterLoaded, filters]);
        return { ...list, filterLoaded };
      },
      { wrapper: createWrapper(client) },
    );

    expect(restored.result.current.filterLoaded).toBe(false);
    expect(restored.result.current.filterValue).toEqual(AppliedFilter);
    await act(async () => complete());
    if (outcome === 'empty') {
      await waitFor(() =>
        expect(restored.result.current.filterLoaded).toBe(true),
      );
      expect(restored.result.current.filterValue).toEqual({});
      await waitFor(() =>
        expect(listDocument).toHaveBeenLastCalledWith(expect.anything(), {
          suffix: undefined,
          run_status: undefined,
          return_empty_metadata: false,
          metadata: undefined,
        }),
      );
      restored.unmount();
      expect(mountList().result.current.filterValue).toEqual({});
    } else {
      await waitFor(() =>
        expect(
          client.getQueryState(DocumentKeys.filter('', mockDatasetId))?.status,
        ).toBe('error'),
      );
      expect(restored.result.current.filterLoaded).toBe(false);
      expect(restored.result.current.filterValue).toEqual(AppliedFilter);
      restored.unmount();
      expect(mountList().result.current.filterValue).toEqual(AppliedFilter);
    }
  },
);

it('restores the latest search on a rapid round trip before debounce completes', async () => {
  const list = mountList();
  search(list.result, 'design');
  await waitFor(() =>
    expect(listDocument).toHaveBeenLastCalledWith(
      expect.objectContaining({ keywords: 'design' }),
      expect.anything(),
    ),
  );
  search(list.result, 'updated');
  mockDatasetId = 'dataset-b';
  list.rerender();
  jest.mocked(listDocument).mockClear();
  mockDatasetId = 'dataset-a';
  list.rerender();
  expect(jest.mocked(listDocument).mock.calls[0][0]).toMatchObject({
    id: 'dataset-a',
    keywords: 'updated',
  });
});

it.each([
  '{broken json',
  'null',
  '{"searchString":1,"filterValue":{}}',
  '{"searchString":"design","filterValue":{"run":null}}',
])('ignores invalid stored list preferences: %s', (stored) => {
  const first = mountList();
  search(first.result, 'design');
  const key = sessionStorage.key(0)!;
  first.unmount();
  sessionStorage.setItem(key, stored);

  const restored = mountList();
  expect(restored.result.current.searchString).toBe('');
  expect(restored.result.current.filterValue).toEqual({});
});

it("keeps each user's list preferences separate", () => {
  const first = mountList();
  search(first.result, 'design');
  act(() => first.result.current.handleFilterSubmit(AppliedFilter));
  first.unmount();
  localStorage.setItem(
    'userInfo',
    JSON.stringify({ email: 'grace@example.com' }),
  );

  const otherUser = mountList();
  expect(otherUser.result.current.searchString).toBe('');
  expect(otherUser.result.current.filterValue).toEqual({});
  search(otherUser.result, 'manual');
  otherUser.unmount();
  localStorage.setItem(
    'userInfo',
    JSON.stringify({ email: 'ada@example.com' }),
  );

  const originalUser = mountList();
  expect(originalUser.result.current.searchString).toBe('design');
  expect(originalUser.result.current.filterValue).toEqual(AppliedFilter);
});

it('keeps search and filters usable when session storage is unavailable', () => {
  const blocked = jest
    .spyOn(window, 'sessionStorage', 'get')
    .mockImplementation(() => {
      throw new Error('Storage is blocked');
    });
  try {
    const list = mountList();
    search(list.result, 'design');
    act(() => list.result.current.handleFilterSubmit(AppliedFilter));
    expect(list.result.current.searchString).toBe('design');
    expect(list.result.current.filterValue).toEqual(AppliedFilter);
  } finally {
    blocked.mockRestore();
  }
});

it("does not reuse another dataset's filter catalog on parameter routes", async () => {
  jest.mocked(documentFilter).mockImplementation(
    async (id) =>
      ({
        data: {
          code: 0,
          data: {
            filter: {
              suffix: id === 'dataset-a' ? { pdf: 1 } : { docx: 1 },
              metadata: {},
              ingestion_status: {},
            },
          },
        },
      }) as never,
  );
  const catalog = renderHook(() => useGetDocumentFilter(), {
    wrapper: createWrapper(),
  });
  await waitFor(() =>
    expect(catalog.result.current.filter.suffix).toHaveProperty('pdf'),
  );

  mockDatasetId = 'dataset-b';
  catalog.rerender();
  expect(catalog.result.current.filter.suffix).not.toHaveProperty('pdf');
  await waitFor(() =>
    expect(catalog.result.current.filter.suffix).toHaveProperty('docx'),
  );

  mockDatasetId = 'dataset-a';
  catalog.rerender();
  expect(catalog.result.current.filter.suffix).not.toHaveProperty('docx');
  await waitFor(() =>
    expect(catalog.result.current.filter.suffix).toHaveProperty('pdf'),
  );
});
