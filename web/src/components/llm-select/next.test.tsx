import { render, screen } from '@testing-library/react';
import { NextLLMSelect } from './next';

const mockUseModelValidIds = jest.fn();
const mockT = (key: string) => key;

jest.mock('@/hooks/use-llm-request', () => ({
  useModelValidIds: (...args: unknown[]) => mockUseModelValidIds(...args),
}));

jest.mock('@/constants/knowledge', () => ({
  LlmModelType: { Chat: 'chat', Image2text: 'image2text' },
}));

jest.mock('../model-tree-select', () => ({
  ModelTypeMap: { llm_id: ['chat', 'vlm'] },
}));

jest.mock('@/ee/components/model-reference-label', () => ({
  ModelReferenceLabel: ({ value }: { value?: string }) => (
    <span data-testid="model-reference-label">{value}</span>
  ),
}));

jest.mock('./llm-label', () => ({
  MissingModelLabel: ({ value }: { value?: string }) => (
    <span data-testid="missing-model-label">{value}</span>
  ),
}));

jest.mock('../llm-setting-items/next', () => ({
  LlmSettingFieldItems: () => null,
}));

jest.mock('react-i18next', () => ({
  ...jest.requireActual('react-i18next'),
  useTranslation: () => ({ t: mockT }),
}));

const groupRef = 'gp-0123456789abcdef0123456789abcdef';
const pendingMessage = 'setting.modelGroupSelector.pending';

describe('NextLLMSelect', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('shows group pending in the trigger and missing only after it settles', () => {
    mockUseModelValidIds.mockReturnValue({
      validIds: new Set(),
      isFetched: true,
      getPendingMessage: (value: string) =>
        value === groupRef ? pendingMessage : undefined,
    });
    const { rerender } = render(
      <NextLLMSelect value={groupRef} triggerTestId="llm-trigger" />,
    );

    expect(screen.getByTestId('model-reference-label')).toHaveTextContent(
      groupRef,
    );
    expect(screen.queryByTestId('missing-model-label')).toBeNull();
    expect(screen.getByTestId('llm-trigger')).toHaveAttribute(
      'title',
      pendingMessage,
    );

    mockUseModelValidIds.mockReturnValue({
      validIds: new Set(),
      isFetched: true,
      getPendingMessage: () => undefined,
    });
    rerender(
      <NextLLMSelect
        value={groupRef}
        triggerTestId="llm-trigger"
        optionTestIdPrefix="model-option"
      />,
    );

    expect(screen.getByTestId('missing-model-label')).toHaveTextContent(
      groupRef,
    );
    expect(screen.queryByTestId('model-reference-label')).toBeNull();
    expect(screen.getByTestId('llm-trigger')).not.toHaveAttribute(
      'title',
      pendingMessage,
    );
  });

  it('keeps a usable direct model on the normal label path', () => {
    mockUseModelValidIds.mockReturnValue({
      validIds: new Set(['model-1']),
      isFetched: true,
      getPendingMessage: () => undefined,
    });
    render(<NextLLMSelect value="model-1" triggerTestId="llm-trigger" />);

    expect(screen.getByTestId('model-reference-label')).toHaveTextContent(
      'model-1',
    );
    expect(screen.queryByTestId('missing-model-label')).toBeNull();
    expect(screen.getByTestId('llm-trigger')).not.toHaveAttribute('title');
  });
});
