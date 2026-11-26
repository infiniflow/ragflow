import { fireEvent, render, screen } from '@testing-library/react';
import { ConversationList } from './conversation-list';

const conversations = [
  { id: 'session-1', name: 'Alpha' },
  { id: 'session-2', name: 'Beta', is_new: true },
];

describe('ConversationList', () => {
  it('marks the selected row and truncates conversation names', () => {
    render(
      <ConversationList
        items={conversations}
        label="Conversations"
        selectedId="session-1"
        onSelect={jest.fn()}
      />,
    );

    const selectedButton = screen.getByRole('button', { name: 'Alpha' });
    expect(selectedButton.closest('li')).toHaveAttribute(
      'data-selected',
      'true',
    );
    expect(selectedButton).toHaveAttribute('aria-current', 'page');
    expect(selectedButton).toHaveClass('min-w-0', 'truncate');
    expect(
      screen.getByRole('button', { name: 'Beta' }).closest('li'),
    ).toHaveAttribute('data-selected', 'false');
  });

  it('returns the session id and temporary state when selected', () => {
    const onSelect = jest.fn();
    render(
      <ConversationList
        items={conversations}
        label="Conversations"
        onSelect={onSelect}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Beta' }));

    expect(onSelect).toHaveBeenCalledWith('session-2', true);
  });

  it('lets the caller wrap the shared action trigger', () => {
    render(
      <ConversationList
        items={conversations}
        label="Conversations"
        onSelect={jest.fn()}
        renderActions={(item, trigger) => (
          <div data-testid={`actions-${item.id}`}>{trigger}</div>
        )}
      />,
    );

    expect(screen.getByTestId('actions-session-1')).toContainElement(
      screen.getAllByTestId('conversation-list-actions')[0],
    );
    expect(screen.getByTestId('actions-session-2')).toContainElement(
      screen.getAllByTestId('conversation-list-actions')[1],
    );
  });
});
