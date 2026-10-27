import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import CategorizeForm from './index';

const mockUpdateNodeForm = jest.fn();
const mockModels = [
  {
    model_id: 'chat-id',
    model_type: ['chat'],
    name: 'chat-model',
    provider_name: 'OpenAI',
    instance_name: 'default',
  },
  {
    model_id: 'decision-id',
    model_type: ['decision'],
    name: 'decision-model-preview',
    provider_name: 'SystemOne',
    instance_name: 'default',
  },
];
const mockFetchModels = jest.fn().mockReturnValue({
  data: mockModels,
  isFetched: true,
  isError: false,
  loading: false,
});

jest.mock('@/hooks/use-llm-request', () => ({
  useFetchAllAddedModels: (...args: unknown[]) => mockFetchModels(...args),
}));
jest.mock('@/components/svg-icon', () => ({ LlmIcon: () => null }));
jest.mock('@/components/llm-select/llm-label', () => ({
  __esModule: true,
  default: ({ value }: { value: string }) => <span>{value}</span>,
  MissingModelLabel: ({ value }: { value: string }) => (
    <span>Missing: {value}</span>
  ),
}));
jest.mock('@/components/llm-setting-items/next', () => ({
  LlmSettingSchema: { llm_id: require('zod').z.string() },
}));
jest.mock('@/components/message-history-window-size-item', () => ({
  MessageHistoryWindowSizeFormField: () => null,
}));
jest.mock('../../constant', () => ({
  initialCategorizeValues: { outputs: {} },
}));
jest.mock('../../context', () => ({ useOwnerTenantId: () => 'canvas-owner' }));
jest.mock('../../store', () => ({
  __esModule: true,
  default: (select: any) => select({ updateNodeForm: mockUpdateNodeForm }),
}));
jest.mock('../components/output', () => ({ Output: () => null }));
jest.mock('../components/query-variable', () => ({
  QueryVariable: () => null,
}));
jest.mock('./dynamic-categorize', () => ({
  __esModule: true,
  default: () => null,
}));

it('offers chat and decision models and saves the selected decision model ID', async () => {
  render(
    <CategorizeForm
      node={
        {
          id: 'Categorize:1',
          data: {
            form: {
              llm_id: 'chat-id',
              message_history_window_size: 0,
              items: [],
            },
          },
        } as never
      }
    />,
  );

  fireEvent.click(
    screen.getByRole('button', { name: 'chat-model@default@OpenAI' }),
  );
  expect(screen.getByText('chat-model')).toBeInTheDocument();
  fireEvent.click(screen.getByText('decision-model-preview'));

  await waitFor(() => {
    expect(mockUpdateNodeForm).toHaveBeenLastCalledWith(
      'Categorize:1',
      expect.objectContaining({ llm_id: 'decision-id' }),
    );
  });
  expect(screen.queryByText('Missing: decision-id')).toBeNull();
  expect(mockFetchModels).toHaveBeenCalledWith(undefined, 'canvas-owner');
});
