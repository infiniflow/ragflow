import { act, fireEvent, render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';

import { GPUStackInstanceCard } from './gpustack-instance-card';

const mockDiscover = jest.fn();
const mockAddInstance = jest.fn();
const mockAddModel = jest.fn();
const mockPatchModel = jest.fn();
const mockUpdateInstance = jest.fn();
let mockInstanceModels: any[] = [];
const loadedModelsState = {
  loading: false,
  isSuccess: true,
  isFetchedAfterMount: true,
};
let mockInstanceModelsState = loadedModelsState;
let mockDiscovering = false;

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: Record<string, unknown>) =>
      options?.count !== undefined ? `${key}:${options.count}` : key,
  }),
}));
jest.mock('@/hooks/use-llm-request', () => ({
  useListProviderModels: () => ({
    loading: mockDiscovering,
    listProviderModels: mockDiscover,
  }),
  useAddProviderInstance: () => ({ addProviderInstance: mockAddInstance }),
  useUpdateProviderInstance: () => ({
    updateProviderInstance: mockUpdateInstance,
  }),
  useAddInstanceModel: () => ({ addInstanceModel: mockAddModel }),
  useDeleteInstanceModels: () => ({ deleteInstanceModels: jest.fn() }),
  usePatchInstanceModel: () => ({ patchInstanceModel: mockPatchModel }),
  useFetchInstanceModels: () => ({
    data: mockInstanceModels,
    ...mockInstanceModelsState,
  }),
}));
jest.mock('../hooks', () => ({
  useFetchInstanceNameSet: () => ({
    instanceNameSet: new Set<string>(['taken']),
  }),
}));
jest.mock('./hooks', () => ({
  useDeleteInstance: () => jest.fn(),
  useLazyInstanceDetails: () => ({
    instanceDetails: { base_url: 'http://gs.local' },
  }),
}));
jest.mock('@/components/confirm-delete-dialog', () => ({
  ConfirmDeleteDialog: ({ children }: any) => children,
}));
jest.mock('@/components/ui/select', () => ({
  Select: ({ onValueChange, children }: any) => (
    <div>
      <button onClick={() => onValueChange('vision')}>make-vision</button>
      {children}
    </div>
  ),
  SelectContent: () => null,
  SelectItem: () => null,
  SelectTrigger: () => null,
  SelectValue: () => null,
}));

const served = [
  { name: 'writer', model_types: ['chat'], max_tokens: 8192 },
  { name: 'encoder', model_types: ['embedding'], max_tokens: 512 },
];
const instance = { id: 'id-1', instance_name: 'saved' } as any;

function fillForm(name = 'my-gpustack') {
  fireEvent.change(screen.getByTestId('gpustack-instance-name'), {
    target: { value: name },
  });
  fireEvent.change(screen.getByTestId('gpustack-base-url'), {
    target: { value: 'http://10.0.0.5:8090' },
  });
  fireEvent.change(screen.getByTestId('gpustack-api-key'), {
    target: { value: 'secret' },
  });
}

async function connect() {
  fillForm();
  await act(async () => {
    fireEvent.click(screen.getByTestId('gpustack-connect'));
  });
}

beforeEach(() => {
  jest.clearAllMocks();
  mockInstanceModels = [];
  mockInstanceModelsState = loadedModelsState;
  mockDiscovering = false;
});

describe('GPUStackInstanceCard draft', () => {
  const renderDraft = (onSaved = jest.fn()) =>
    render(
      <GPUStackInstanceCard
        providerName="GPUStack"
        instance={{ id: '', instance_name: '' } as any}
        isDraft
        onSaved={onSaved}
      />,
    );

  it('needs a name, URL and key before connecting', () => {
    renderDraft();
    expect(screen.getByTestId('gpustack-connect')).toBeDisabled();
    fillForm(' ');
    expect(screen.getByTestId('gpustack-connect')).toBeDisabled();
    fillForm();
    expect(screen.getByTestId('gpustack-connect')).toBeEnabled();
  });

  it('rejects a name that is already in use', () => {
    renderDraft();
    fillForm('taken');
    expect(screen.getByText('instanceNameExists')).toBeInTheDocument();
    expect(screen.getByTestId('gpustack-connect')).toBeDisabled();
  });

  it('cannot add while discovering again', async () => {
    mockDiscover.mockResolvedValue({ code: 0, data: served });
    const draft = () => (
      <GPUStackInstanceCard
        providerName="GPUStack"
        instance={{ id: '', instance_name: '' } as any}
        isDraft
      />
    );
    const { rerender } = render(draft());
    await connect();
    expect(screen.getByTestId('gpustack-add-models')).toBeEnabled();
    mockDiscovering = true;
    rerender(draft());
    expect(screen.getByTestId('gpustack-add-models')).toBeDisabled();
  });

  it('drops a pending result when the credentials change', async () => {
    let resolve: (value: unknown) => void = () => {};
    mockDiscover.mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }),
    );
    renderDraft();
    await connect();
    fireEvent.change(screen.getByTestId('gpustack-api-key'), {
      target: { value: 'other' },
    });
    await act(async () => {
      resolve({ code: 0, data: served });
    });
    expect(screen.queryByTestId('gpustack-add-models')).toBeNull();
  });

  it('lists models with the entered credential, keeping failures local', async () => {
    mockDiscover.mockResolvedValue({ code: 0, data: served });
    renderDraft();
    await connect();
    expect(mockDiscover).toHaveBeenCalledWith({
      provider_name: 'GPUStack',
      skipGlobalErrorNotification: true,
      api_key: 'secret',
      base_url: 'http://10.0.0.5:8090',
    });
  });

  it('adds every discovered model in one step by default', async () => {
    mockDiscover.mockResolvedValue({ code: 0, data: served });
    mockAddInstance.mockResolvedValue({ code: 0 });
    const onSaved = jest.fn();
    renderDraft(onSaved);
    await connect();

    expect(screen.getByTestId('gpustack-group-chat')).toBeInTheDocument();
    expect(screen.getByTestId('gpustack-group-embedding')).toBeInTheDocument();
    await act(async () => {
      fireEvent.click(screen.getByTestId('gpustack-add-models'));
    });

    expect(mockAddInstance).toHaveBeenCalledWith({
      llm_factory: 'GPUStack',
      instance_name: 'my-gpustack',
      api_key: 'secret',
      base_url: 'http://10.0.0.5:8090',
      model_info: [
        { model_name: 'writer', model_type: ['chat'], max_tokens: 8192 },
        { model_name: 'encoder', model_type: ['embedding'], max_tokens: 512 },
      ],
    });
    expect(onSaved).toHaveBeenCalled();
  });

  it('leaves out models the user unchecks', async () => {
    mockDiscover.mockResolvedValue({ code: 0, data: served });
    mockAddInstance.mockResolvedValue({ code: 0 });
    renderDraft();
    await connect();
    fireEvent.click(screen.getAllByLabelText('gpustack.select')[0]);
    expect(screen.getByTestId('gpustack-add-models')).toHaveTextContent(
      'gpustack.addModels:1',
    );
  });

  it('explains a rejected key next to the form', async () => {
    mockDiscover.mockResolvedValue({
      code: 102,
      message: 'API request failed with status 401: unauthorized',
    });
    renderDraft();
    await connect();
    expect(screen.getByTestId('gpustack-error')).toHaveTextContent(
      'gpustack.errorAuth',
    );
    expect(screen.queryByTestId('gpustack-add-models')).toBeNull();
  });

  it('explains how to get models when none are running', async () => {
    mockDiscover.mockResolvedValue({ code: 0, data: [] });
    renderDraft();
    await connect();
    expect(screen.getByTestId('gpustack-empty')).toBeInTheDocument();
    expect(screen.queryByTestId('gpustack-add-models')).toBeNull();
  });

  it('hides the context length of audio models', async () => {
    mockDiscover.mockResolvedValue({
      code: 0,
      data: [
        { name: 'writer', model_types: ['chat'], max_tokens: 4096 },
        { name: 'voice', model_types: ['tts'], max_tokens: 0 },
      ],
    });
    renderDraft();
    await connect();
    expect(screen.getAllByText(/^gpustack\.tokens/)).toHaveLength(1);
  });

  it('warns about localhost', () => {
    renderDraft();
    fireEvent.change(screen.getByTestId('gpustack-base-url'), {
      target: { value: 'http://localhost:8080' },
    });
    expect(screen.getByText('gpustack.loopbackWarning')).toBeInTheDocument();
  });
});

describe('GPUStackInstanceCard saved', () => {
  it('uses saved credentials and shows what changed on the server', async () => {
    mockInstanceModels = [
      { name: 'writer', model_type: ['chat'], max_tokens: 8192 },
      { name: 'stopped', model_type: ['chat'], max_tokens: 8192 },
    ];
    mockDiscover.mockResolvedValue({ code: 0, data: served });
    mockAddModel.mockResolvedValue({ code: 0 });
    await act(async () => {
      render(
        <GPUStackInstanceCard
          providerName="GPUStack"
          instance={instance}
          defaultOpen
        />,
      );
    });

    expect(mockDiscover).toHaveBeenCalledWith({
      provider_name: 'GPUStack',
      skipGlobalErrorNotification: true,
      use_saved_credentials: true,
      instance_name: 'saved',
    });
    expect(screen.getByText('gpustack.notRunning')).toBeInTheDocument();
    await act(async () => {
      fireEvent.click(screen.getByTestId('gpustack-add-selected'));
    });
    expect(mockAddModel).toHaveBeenCalledTimes(1);
    expect(mockAddModel).toHaveBeenCalledWith(
      expect.objectContaining({
        instance_name: 'saved',
        model_name: 'encoder',
        model_type: ['embedding'],
        max_tokens: 512,
      }),
    );
  });

  const savedModels = [
    {
      name: 'writer',
      model_type: ['chat'],
      max_tokens: 8192,
      extra: { is_tools: true },
    },
  ];

  async function renderSaved() {
    mockInstanceModels = savedModels;
    mockDiscover.mockResolvedValue({ code: 0, data: [served[0]] });
    await act(async () => {
      render(
        <GPUStackInstanceCard
          providerName="GPUStack"
          instance={instance}
          defaultOpen
        />,
      );
    });
  }

  it('changes a saved type through the PATCH endpoint, keeping settings', async () => {
    await renderSaved();
    await act(async () => {
      fireEvent.click(screen.getByText('make-vision'));
    });
    expect(mockPatchModel).toHaveBeenCalledWith({
      provider_name: 'GPUStack',
      instance_name: 'saved',
      model_name: 'writer',
      max_tokens: 8192,
      model_type: ['chat', 'vision'],
      extra: { is_tools: true },
    });
  });

  it('keeps the saved models and key when editing the connection', async () => {
    mockUpdateInstance.mockResolvedValue({ code: 0 });
    await renderSaved();
    fireEvent.click(screen.getByTestId('gpustack-edit'));
    await act(async () => {
      fireEvent.click(screen.getByTestId('gpustack-save-connection'));
    });
    const body = mockUpdateInstance.mock.calls[0][0];
    expect(body).toMatchObject({
      id: 'id-1',
      instance_name: 'saved',
      base_url: 'http://gs.local',
      region: 'default',
      model_info: [
        {
          model_name: 'writer',
          model_type: ['chat'],
          max_tokens: 8192,
          extra: { is_tools: true },
        },
      ],
    });
    expect(body).not.toHaveProperty('api_key');
    expect(screen.queryByTestId('gpustack-save-connection')).toBeNull();
  });

  it.each([
    ['loading', { ...loadedModelsState, loading: true }, false],
    [
      'not fetched',
      { ...loadedModelsState, isFetchedAfterMount: false },
      false,
    ],
    ['failed to load', { ...loadedModelsState, isSuccess: false }, true],
  ])(
    'cannot save while the models have %s',
    async (_name, state, explained) => {
      mockInstanceModelsState = state;
      await renderSaved();
      fireEvent.click(screen.getByTestId('gpustack-edit'));
      expect(screen.getByTestId('gpustack-save-connection')).toBeDisabled();
      expect(!!screen.queryByTestId('gpustack-models-load-failed')).toBe(
        explained,
      );
    },
  );
});
