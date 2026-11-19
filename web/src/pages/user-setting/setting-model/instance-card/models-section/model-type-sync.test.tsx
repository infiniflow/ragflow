import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  act,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from '@testing-library/react';
import { ModelTypeSyncDialog } from './components/model-type-sync-dialog';
import { useModelTypeSync } from './hooks';

type PropsWithChildren = import('react').PropsWithChildren;
type IInstanceModel = import('@/interfaces/database/llm').IInstanceModel;
type IProviderModelItem = import('@/interfaces/request/llm').IProviderModelItem;

const mockPatchInstanceModel = jest.fn();
const mockSuccess = jest.fn();
function mockInstanceModelsKey(providerName: string, instanceName: string) {
  return ['addedProviders', providerName, instanceName, 'models'] as const;
}
jest.mock('@/hooks/use-llm-request', () => ({
  LlmKeys: {
    instanceModels: mockInstanceModelsKey,
    addedProviders: () => ['addedProviders'],
    allModels: () => ['allModels'],
    defaultModels: () => ['defaultModels'],
  },
}));
jest.mock('@/components/dynamic-form', () => ({
  FormFieldType: new Proxy({}, { get: (_target, key) => String(key) }),
}));
jest.mock('../../provider-schema/hooks', () => ({
  useProviderFields: jest.fn(),
}));
jest.mock('../use-custom-model-fields', () => ({
  useCustomModelFields: () => [],
}));
jest.mock('@/services/llm-service', () => ({
  __esModule: true,
  default: {
    patchInstanceModel: (...args: unknown[]) => mockPatchInstanceModel(...args),
  },
}));
jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { success: (...args: unknown[]) => mockSuccess(...args) },
}));
jest.mock('../available-models', () => ({
  sortModelTypes: (types: string[]) => [...types].sort(),
  mapModelKey: { chat: 'LLM', vision: 'VLM', embedding: 'Embedding' },
}));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, values?: Record<string, number>) => {
      const labels: Record<string, string> = {
        'setting.syncModelTypes': 'Sync model types',
        'setting.syncModelTypesDescription': 'Confirm the listed type changes.',
        'setting.syncModelTypesPartial':
          '{{success}} succeeded, {{failed}} failed',
        'setting.syncModelTypesSuccess': 'Updated {{count}} model types',
        'common.cancel': 'Cancel',
        'common.confirm': 'Confirm',
      };
      return (labels[key] || key).replace(
        /\{\{(\w+)\}\}/g,
        (_match, name: string) => String(values?.[name]),
      );
    },
  }),
}));

const ProviderName = 'NovaCode';
const InstanceName = 'nova-instance';
const ModelKey = mockInstanceModelsKey(ProviderName, InstanceName);

function catalogModel(
  name: string,
  modelTypes: string[] = ['chat', 'vision'],
): IProviderModelItem {
  return { name, max_tokens: 1000000, model_types: modelTypes, features: [] };
}
function savedModel(name: string, modelTypes = ['chat']): IInstanceModel {
  return {
    name,
    model_type: modelTypes,
    max_tokens: 32768,
    status: 'inactive',
    verify: 'success',
    is_tools: false,
    extra: { is_tools: false, custom_option: 'preserved' },
  };
}
function setup(
  saved: IInstanceModel[],
  catalog = saved.map((item) => catalogModel(item.name)),
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  queryClient.setQueryData(ModelKey, saved);
  const invalidate = jest.spyOn(queryClient, 'invalidateQueries');
  const wrapper = ({ children }: PropsWithChildren) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
  const props = {
    providerName: ProviderName,
    instanceName: InstanceName,
    isDraftInstance: false,
    catalog,
    instanceItems: saved.map((item) => ({
      ...catalogModel(item.name, item.model_type),
      max_tokens: item.max_tokens,
      extra: item.extra,
    })),
  };
  return { queryClient, invalidate, wrapper, props };
}
function SyncHarness(props: ReturnType<typeof setup>['props']) {
  const sync = useModelTypeSync(props);
  return (
    <>
      <button onClick={sync.openTypeSyncPreview}>Preview changes</button>
      <ModelTypeSyncDialog
        open={sync.typeSyncOpen}
        onOpenChange={sync.setTypeSyncOpen}
        changes={sync.typeSyncChanges}
        result={sync.typeSyncResult}
        syncing={sync.typeSyncing}
        onConfirm={sync.confirmTypeSync}
      />
    </>
  );
}

describe('explicit model type sync', () => {
  beforeEach(() => {
    mockPatchInstanceModel.mockReset().mockResolvedValue({ data: { code: 0 } });
    mockSuccess.mockClear();
  });

  it('previews all eleven changes and sends nothing until confirmation', async () => {
    const saved = Array.from({ length: 11 }, (_, index) =>
      savedModel(`gpt-model-${index}`),
    );
    const { wrapper, props, queryClient, invalidate } = setup(saved);
    render(<SyncHarness {...props} />, { wrapper });
    expect(mockPatchInstanceModel).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Preview changes' }));
    expect(screen.getAllByText('LLM → LLM, VLM')).toHaveLength(11);
    expect(mockPatchInstanceModel).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(mockPatchInstanceModel).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Preview changes' }));
    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(mockSuccess).toHaveBeenCalledTimes(1));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(mockPatchInstanceModel).toHaveBeenCalledTimes(11);
    saved.forEach((item, index) => {
      expect(mockPatchInstanceModel).toHaveBeenNthCalledWith(index + 1, {
        provider_name: ProviderName,
        instance_name: InstanceName,
        model_name: item.name,
        model_type: ['chat', 'vision'],
      });
    });
    expect(queryClient.getQueryData(ModelKey)).toEqual(
      saved.map((item) => ({ ...item, model_type: ['chat', 'vision'] })),
    );
    expect(invalidate).toHaveBeenCalledTimes(4);
  });

  it('leaves unmatched, already-correct and empty-type catalog models unchanged', () => {
    const saved = [
      savedModel('custom-alias'),
      savedModel('same', ['vision', 'chat']),
      savedModel('empty'),
    ];
    const { wrapper, props } = setup(saved, [
      catalogModel('custom'),
      catalogModel('same', ['chat', 'vision']),
      catalogModel('empty', []),
      catalogModel('new-model'),
    ]);
    const { result } = renderHook(() => useModelTypeSync(props), { wrapper });
    expect(result.current.typeSyncAvailable).toBe(false);
    act(() => result.current.openTypeSyncPreview());
    expect(result.current.typeSyncOpen).toBe(false);
    expect(mockPatchInstanceModel).not.toHaveBeenCalled();
  });

  it.each([false, true])(
    'does not sync an empty catalog (draft: %s)',
    (draft) => {
      const { wrapper, props } = setup([savedModel('gpt-6.1-sol')], []);
      const { result } = renderHook(
        () => useModelTypeSync({ ...props, isDraftInstance: draft }),
        { wrapper },
      );
      act(() => result.current.openTypeSyncPreview());
      expect(result.current.typeSyncAvailable).toBe(false);
      expect(result.current.typeSyncOpen).toBe(false);
      expect(mockPatchInstanceModel).not.toHaveBeenCalled();
    },
  );

  it('does not offer saved-model synchronization for a draft with catalog changes', () => {
    const { wrapper, props } = setup([savedModel('gpt-6.1-sol')]);
    const { result } = renderHook(
      () => useModelTypeSync({ ...props, isDraftInstance: true }),
      { wrapper },
    );
    expect(result.current.typeSyncAvailable).toBe(false);
  });

  it('freezes the preview when the catalog changes before confirmation', async () => {
    const { wrapper, props } = setup([savedModel('gpt-6.1-sol')]);
    const { result, rerender } = renderHook(
      (currentProps) => useModelTypeSync(currentProps),
      { wrapper, initialProps: props },
    );
    act(() => result.current.openTypeSyncPreview());
    rerender({
      ...props,
      catalog: [catalogModel('gpt-6.1-sol', ['embedding'])],
    });
    expect(result.current.typeSyncChanges[0].nextTypes).toEqual([
      'chat',
      'vision',
    ]);
    await act(() => result.current.confirmTypeSync());
    expect(mockPatchInstanceModel).toHaveBeenCalledWith({
      provider_name: ProviderName,
      instance_name: InstanceName,
      model_name: 'gpt-6.1-sol',
      model_type: ['chat', 'vision'],
    });
  });

  it('keeps failed models for explicit retry and preserves every other saved field', async () => {
    const saved = [
      savedModel('success'),
      savedModel('api-error'),
      savedModel('network-error'),
    ];
    const { wrapper, props, queryClient } = setup(saved);
    mockPatchInstanceModel
      .mockResolvedValueOnce({ data: { code: 0 } })
      .mockResolvedValueOnce({ data: { code: 500 } })
      .mockRejectedValueOnce(new Error('offline'));
    const { result } = renderHook(() => useModelTypeSync(props), { wrapper });
    act(() => result.current.openTypeSyncPreview());
    await act(() => result.current.confirmTypeSync());
    expect(result.current.typeSyncOpen).toBe(true);
    expect(result.current.typeSyncResult).toEqual({ success: 1, failed: 2 });
    expect(result.current.typeSyncChanges.map((item) => item.name)).toEqual([
      'api-error',
      'network-error',
    ]);
    expect(queryClient.getQueryData(ModelKey)).toEqual([
      { ...saved[0], model_type: ['chat', 'vision'] },
      saved[1],
      saved[2],
    ]);
    expect(mockSuccess).not.toHaveBeenCalled();
    await act(() => result.current.confirmTypeSync());
    expect(mockPatchInstanceModel).toHaveBeenCalledTimes(5);
    expect(
      mockPatchInstanceModel.mock.calls
        .slice(3)
        .map(([params]) => params.model_name),
    ).toEqual(['api-error', 'network-error']);
    expect(result.current.typeSyncOpen).toBe(false);
    expect(mockSuccess).toHaveBeenCalledTimes(1);
  });

  it('does not close or submit twice while a confirmed sync is pending', async () => {
    let resolvePatch: (value: { data: { code: number } }) => void = () => {};
    mockPatchInstanceModel.mockReturnValue(
      new Promise((resolve) => {
        resolvePatch = resolve;
      }),
    );
    const { wrapper, props } = setup([savedModel('gpt-6.1-sol')]);
    const { result } = renderHook(() => useModelTypeSync(props), { wrapper });
    act(() => result.current.openTypeSyncPreview());
    let pending: Promise<void>;
    act(() => {
      pending = result.current.confirmTypeSync();
    });
    await waitFor(() => expect(result.current.typeSyncing).toBe(true));
    act(() => result.current.setTypeSyncOpen(false));
    await act(() => result.current.confirmTypeSync());
    expect(result.current.typeSyncOpen).toBe(true);
    expect(mockPatchInstanceModel).toHaveBeenCalledTimes(1);
    await act(async () => {
      resolvePatch({ data: { code: 0 } });
      await pending;
    });
    expect(result.current.typeSyncOpen).toBe(false);
  });
});
