import { fireEvent, render, screen } from '@testing-library/react';

import { ViewMode, ViewModeLabelKeyMap, VisibleViewModes } from './constants';
import { ViewModeNav } from './view-mode-nav';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

describe('ViewModeNav', () => {
  const defaultProps = {
    viewMode: ViewMode.LlmWiki,
    open: true,
    onOpenChange: jest.fn(),
    onViewModeChange: jest.fn(),
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders every existing view option and marks the current view', () => {
    render(<ViewModeNav {...defaultProps} />);

    VisibleViewModes.forEach((mode) => {
      expect(
        screen.getByRole('button', { name: ViewModeLabelKeyMap[mode] }),
      ).toBeInTheDocument();
    });
    expect(
      screen.getByRole('button', {
        name: ViewModeLabelKeyMap[ViewMode.LlmWiki],
      }),
    ).toHaveAttribute('aria-pressed', 'true');
    expect(
      screen.getByRole('button', {
        name: ViewModeLabelKeyMap[ViewMode.Graph],
      }),
    ).toHaveAttribute('aria-pressed', 'false');
  });

  it('selects the clicked view', () => {
    render(<ViewModeNav {...defaultProps} />);

    fireEvent.click(
      screen.getByRole('button', {
        name: ViewModeLabelKeyMap[ViewMode.Graph],
      }),
    );

    expect(defaultProps.onViewModeChange).toHaveBeenCalledWith(ViewMode.Graph);
  });

  it('collapses the options and reports expansion changes', () => {
    const { rerender } = render(<ViewModeNav {...defaultProps} />);
    const artifactsTrigger = screen.getByRole('button', {
      name: 'knowledgeCompilation.artifacts',
    });

    fireEvent.click(artifactsTrigger);
    expect(defaultProps.onOpenChange).toHaveBeenCalledWith(false);

    rerender(<ViewModeNav {...defaultProps} open={false} />);
    expect(
      screen.queryByRole('button', {
        name: ViewModeLabelKeyMap[ViewMode.LlmWiki],
      }),
    ).not.toBeInTheDocument();

    fireEvent.click(artifactsTrigger);
    expect(defaultProps.onOpenChange).toHaveBeenCalledWith(true);
  });
});
