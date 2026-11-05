import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import agentService, { updateVersion } from '@/services/agent-service';
import {
  AgentKeys,
  useDeleteAgent,
  useDeleteVersion,
  useSaveAgent,
  useUpdateAgentSetting,
  useUpdateVersion,
} from '../use-agent-request';
import {
  PipelineVersionKeys,
  useFetchPipelineVersionCatalog,
  useFetchPipelineVersionList,
} from '../use-pipeline-version-request';

jest.mock('@/services/agent-service', () => ({
  __esModule: true,
  default: {
    listAgentVersions: jest.fn(),
    deleteVersion: jest.fn(),
    deleteAgent: jest.fn(),
  },
  updateVersion: jest.fn(),
  updateAgent: jest.fn(),
}));

import { updateAgent } from '@/services/agent-service';

const Catalog = [
  {
    id: 'pipe-1',
    name: 'Invoices',
    versions: [{ id: 'v2', name: 'Version two', alias: 'production' }],
  },
];

function setup() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <MemoryRouter>
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    </MemoryRouter>
  );
  return { client, wrapper };
}

beforeEach(() => {
  jest.clearAllMocks();
  jest
    .mocked(agentService.listAgentVersions)
    .mockResolvedValue({ data: { code: 0, data: Catalog, total: 1 } } as any);
});

it('shares one catalog request across the tree and selected version observers', async () => {
  const { wrapper } = setup();
  const { result } = renderHook(
    () => ({
      catalog: useFetchPipelineVersionCatalog(),
      selected: useFetchPipelineVersionList('pipe-1'),
      missing: useFetchPipelineVersionList('deleted-pipeline'),
    }),
    { wrapper },
  );
  await waitFor(() => expect(result.current.catalog.data).toEqual(Catalog));
  expect(result.current.selected.data).toEqual(Catalog[0].versions);
  expect(result.current.missing.data).toEqual([]);
  expect(agentService.listAgentVersions).toHaveBeenCalledTimes(1);
  expect(agentService.listAgentVersions).toHaveBeenCalledWith(
    { params: { category: 'dataflow' } },
    true,
  );
});

it('reuses the fresh catalog after unmounting and reopening the picker', async () => {
  const { wrapper } = setup();
  const first = renderHook(() => useFetchPipelineVersionCatalog(), { wrapper });
  await waitFor(() => expect(first.result.current.data).toEqual(Catalog));
  first.unmount();
  const second = renderHook(() => useFetchPipelineVersionCatalog(), {
    wrapper,
  });
  expect(second.result.current.data).toEqual(Catalog);
  expect(agentService.listAgentVersions).toHaveBeenCalledTimes(1);
});

it('distinguishes pending data from an empty version list', async () => {
  let resolveFetch!: (value: any) => void;
  jest.mocked(agentService.listAgentVersions).mockImplementation(
    () =>
      new Promise((resolve) => {
        resolveFetch = resolve;
      }),
  );
  const { wrapper } = setup();
  const { result } = renderHook(() => useFetchPipelineVersionList('pipe-1'), {
    wrapper,
  });
  expect(result.current.data).toBeUndefined();
  expect(result.current.loading).toBe(true);
  await act(async () => resolveFetch({ data: { code: 0, data: [] } }));
  await waitFor(() => expect(result.current.data).toEqual([]));
});

it('does not fetch the catalog for an absent selected pipeline', () => {
  const { wrapper } = setup();
  const { result } = renderHook(() => useFetchPipelineVersionList(), {
    wrapper,
  });
  expect(result.current.data).toBeUndefined();
  expect(agentService.listAgentVersions).not.toHaveBeenCalled();
});

it('surfaces API failures instead of caching them as an empty catalog', async () => {
  jest.mocked(agentService.listAgentVersions).mockResolvedValue({
    data: { code: 500, message: 'Failed to load catalog' },
  } as any);
  const { wrapper } = setup();
  const { result } = renderHook(() => useFetchPipelineVersionCatalog(), {
    wrapper,
  });
  await waitFor(() => expect(result.current.isError).toBe(true));
  expect(result.current.data).toBeUndefined();
});

it.each(['alias', 'deleteVersion', 'deletePipeline', 'save', 'rename'])(
  'invalidates cached catalog metadata after %s',
  async (action) => {
    const { client, wrapper } = setup();
    client.setQueryData(PipelineVersionKeys.catalog(), Catalog);
    client.setQueryData(AgentKeys.canvasVersionList('pipe-1'), [
      { id: 'v2', title: 'Version two', alias: 'production' },
    ]);
    jest.mocked(updateVersion).mockResolvedValue({ data: { code: 0 } } as any);
    jest.mocked(updateAgent).mockResolvedValue({ data: { code: 0 } } as any);
    jest
      .mocked(agentService.deleteVersion)
      .mockResolvedValue({ data: { code: 0 } } as any);
    jest
      .mocked(agentService.deleteAgent)
      .mockResolvedValue({ data: { code: 0 } } as any);
    const { result } = renderHook(
      () => ({
        alias: useUpdateVersion(),
        deleteVersion: useDeleteVersion(),
        deletePipeline: useDeleteAgent(),
        save: useSaveAgent(false),
        rename: useUpdateAgentSetting(),
      }),
      { wrapper },
    );
    await act(async () => {
      if (action === 'alias')
        await result.current.alias.updateVersion({
          canvasId: 'pipe-1',
          versionId: 'v2',
          alias: 'new alias',
        });
      if (action === 'deleteVersion')
        await result.current.deleteVersion.deleteVersion({
          canvasId: 'pipe-1',
          versionId: 'v2',
        });
      if (action === 'deletePipeline')
        await result.current.deletePipeline.deleteAgent('pipe-1');
      if (action === 'save')
        await result.current.save.saveAgent({ id: 'pipe-1', release: 'true' });
      if (action === 'rename')
        await result.current.rename.updateAgentSetting({
          id: 'pipe-1',
          title: 'Renamed',
        });
    });
    expect(
      client.getQueryState(PipelineVersionKeys.catalog())?.isInvalidated,
    ).toBe(true);
    if (action === 'alias' || action === 'deleteVersion') {
      expect(AgentKeys.canvasVersionList('pipe-1')).toEqual([
        'canvasVersionList',
        'pipe-1',
      ]);
      expect(
        client.getQueryState(AgentKeys.canvasVersionList('pipe-1'))
          ?.isInvalidated,
      ).toBe(true);
    }
  },
);
