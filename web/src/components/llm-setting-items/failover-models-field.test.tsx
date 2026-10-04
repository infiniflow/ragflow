import { render, screen, waitFor } from '@testing-library/react';
import { TooltipProvider } from '@/components/ui/tooltip';
import { FormProvider, useForm } from 'react-hook-form';
import { FailoverModelsField } from './failover-models-field';

const models: any[] = [
  {
    model_id: 'primary-id',
    name: 'gpt-4o',
    instance_name: 'openai-main',
    provider_name: 'OpenAI',
    model_type: ['chat'],
  },
  {
    model_id: 'backup-id',
    name: 'deepseek-chat',
    instance_name: 'deepseek-cn',
    provider_name: 'DeepInfra',
    model_type: ['chat'],
  },
];

jest.mock('@/hooks/use-llm-request', () => ({
  useFetchAllAddedModels: () => ({ data: models, isFetched: true }),
  useFindLlmByUuid: () => (id: string) => models.find((m) => m.model_id === id),
}));

function Harness({ value }: { value: string[] }) {
  const form = useForm({ defaultValues: { failover_llm_ids: value } });
  return (
    <FormProvider {...form}>
      <form>
        <TooltipProvider>
          <FailoverModelsField
            ownerTenantId="tenant-1"
            primaryLlmId="primary-id"
          />
        </TooltipProvider>
      </form>
    </FormProvider>
  );
}

describe('FailoverModels display', () => {
  it('shows the primary as name + instance rather than a raw id', async () => {
    render(<Harness value={[]} />);

    await waitFor(() => {
      expect(screen.getByText('gpt-4o')).toBeInTheDocument();
    });
    expect(screen.getByText('openai-main')).toBeInTheDocument();
    expect(screen.queryByText('primary-id')).not.toBeInTheDocument();
  });

  it('shows each member as name + instance, in order', async () => {
    render(<Harness value={['backup-id']} />);

    await waitFor(() => {
      expect(screen.getByText('deepseek-chat')).toBeInTheDocument();
    });
    expect(screen.getByText('deepseek-cn')).toBeInTheDocument();
    expect(screen.queryByText('backup-id')).not.toBeInTheDocument();
  });

  it('marks a member the backend can no longer resolve as stale, keeping the row', async () => {
    // The backend skips an unresolvable member with a warning rather than
    // failing the turn, so the row must stay visible for the author to remove.
    render(<Harness value={['deleted-id']} />);

    await waitFor(() => {
      expect(screen.getByTestId('chat-failover-member-0')).toBeInTheDocument();
    });
    expect(screen.getByText('deleted-id')).toBeInTheDocument();
  });

  it('never lists the primary as a fallback member', async () => {
    render(<Harness value={['primary-id', 'backup-id']} />);

    await waitFor(() => {
      expect(screen.getByTestId('chat-failover-member-0')).toBeInTheDocument();
    });
    // Only the backup survives; the primary duplicate is dropped.
    expect(screen.getAllByTestId(/chat-failover-member-/)).toHaveLength(1);
  });
});
