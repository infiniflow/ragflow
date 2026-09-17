import { Form } from '@/components/ui/form';
import { useBuildSwitchOperatorOptions } from '@/hooks/logic-hooks/use-build-operator-options';
import { useFetchKnowledgeMetadata } from '@/hooks/use-knowledge-request';
import { render, screen } from '@testing-library/react';
import { useForm } from 'react-hook-form';
import { MetadataFilterConditions } from './metadata-filter-conditions';

jest.mock('@/hooks/logic-hooks/use-build-operator-options', () => ({
  useBuildSwitchOperatorOptions: jest.fn(),
}));

jest.mock('@/hooks/use-knowledge-request', () => ({
  useFetchKnowledgeMetadata: jest.fn(),
}));

jest.mock('@/pages/agent/form/components/prompt-editor', () => ({
  PromptEditor: () => null,
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const mockUseBuildSwitchOperatorOptions =
  useBuildSwitchOperatorOptions as jest.Mock;
const mockUseFetchKnowledgeMetadata = useFetchKnowledgeMetadata as jest.Mock;

function Harness({ withCondition = false }: { withCondition?: boolean }) {
  const form = useForm({
    defaultValues: {
      meta_data_filter: {
        manual: withCondition
          ? [{ key: 'title', value: 'example', op: 'equals' }]
          : [],
        logic: 'and',
      },
    },
  });

  return (
    <Form {...form}>
      <MetadataFilterConditions kbIds={[]} />
    </Form>
  );
}

describe('MetadataFilterConditions', () => {
  beforeEach(() => {
    mockUseBuildSwitchOperatorOptions.mockReturnValue([
      { label: 'Equals', value: 'equals' },
    ]);
    mockUseFetchKnowledgeMetadata.mockReturnValue({
      data: { title: { example: 1 } },
    });
  });

  it('uses the add button as the dropdown trigger without nesting buttons', () => {
    render(<Harness />);

    const buttons = screen.getAllByRole('button');
    expect(buttons).toHaveLength(1);
    expect(buttons[0]).toHaveAttribute('type', 'button');
  });

  it('keeps the remove action from submitting the surrounding form', () => {
    const { container } = render(<Harness withCondition />);

    const removeIcon = container.querySelector('svg.lucide-x');
    expect(removeIcon).not.toBeNull();
    expect(removeIcon?.closest('button')).toHaveAttribute('type', 'button');
  });
});
