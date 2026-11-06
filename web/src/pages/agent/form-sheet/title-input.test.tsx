import { fireEvent, render, screen } from '@testing-library/react';
import { TitleInput } from './title-input';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

// The name-change hook writes the canvas store; the read-only assertions only
// care whether an editing entry point is rendered, not what it does.
const mockHandleNameBlur = jest.fn(() => true);
jest.mock('../hooks/use-change-node-name', () => ({
  useHandleNodeNameChange: () => ({
    name: 'My Parser',
    handleNameBlur: mockHandleNameBlur,
    handleNameChange: jest.fn(),
  }),
}));

jest.mock('../hooks/use-is-mcp', () => ({
  useIsMcp: () => false,
}));

const baseNode: any = {
  id: 'Parser:1',
  data: { label: 'Parser', name: 'My Parser' },
};

describe('TitleInput', () => {
  it('renders an edit button that switches to editing mode when editable', () => {
    render(<TitleInput node={baseNode} />);

    expect(screen.getByText('My Parser')).toBeInTheDocument();
    const editButton = screen.getByRole('button');
    fireEvent.click(editButton);
    expect(screen.getByDisplayValue('My Parser')).toBeInTheDocument();
  });

  it('renders plain text without an edit button on a read-only canvas', () => {
    render(<TitleInput node={baseNode} readOnly />);

    expect(screen.getByText('My Parser')).toBeInTheDocument();
    expect(screen.queryByRole('button')).toBeNull();
    // Never enters editing mode.
    expect(screen.queryByDisplayValue('My Parser')).toBeNull();
  });
});
