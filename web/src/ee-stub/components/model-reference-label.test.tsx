import { render, screen } from '@testing-library/react';
import { ModelReferenceLabel } from './model-reference-label';

jest.mock('@/components/llm-select/llm-label', () => ({
  __esModule: true,
  default: ({
    value,
    resourceTenantId,
  }: {
    value?: string;
    resourceTenantId?: string;
  }) => (
    <span data-testid="direct-label" data-owner={resourceTenantId}>
      {value}
    </span>
  ),
}));

describe('OSS ModelReferenceLabel', () => {
  it('delegates group-shaped values to the direct model label', () => {
    const groupRef = 'gp-0123456789abcdef0123456789abcdef';
    render(<ModelReferenceLabel value={groupRef} ownerTenantId="owner-1" />);

    expect(screen.getByTestId('direct-label')).toHaveTextContent(groupRef);
    expect(screen.getByTestId('direct-label')).toHaveAttribute(
      'data-owner',
      'owner-1',
    );
  });
});
