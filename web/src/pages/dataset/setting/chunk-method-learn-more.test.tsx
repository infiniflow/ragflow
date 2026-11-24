import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import ChunkMethodLearnMore from './chunk-method-learn-more';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('./category-panel', () => ({
  __esModule: true,
  default: ({ chunkMethod }: { chunkMethod: string }) => (
    <div data-testid="category-panel">{chunkMethod}</div>
  ),
}));

describe('ChunkMethodLearnMore', () => {
  it('opens the current parser details in a sheet and closes it', async () => {
    render(<ChunkMethodLearnMore parserId="general" />);

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();

    const trigger = screen.getByRole('button', {
      name: 'knowledgeDetails.learnMore',
    });
    expect(trigger).toHaveAttribute('type', 'button');
    fireEvent.click(trigger);

    expect(await screen.findByRole('dialog')).toBeInTheDocument();
    expect(document.querySelector('.fixed.inset-0')).not.toBeInTheDocument();
    expect(
      screen.getByText('knowledgeDetails.builtInParserDetails'),
    ).toBeInTheDocument();
    expect(screen.getByTestId('category-panel')).toHaveTextContent('general');

    fireEvent.click(screen.getByRole('button', { name: 'Close' }));

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
  });
});
