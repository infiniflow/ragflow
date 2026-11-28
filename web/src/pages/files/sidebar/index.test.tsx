import { TooltipProvider } from '@/components/ui/tooltip';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

import { FilesSidebar } from './index';

function renderSidebar(initialEntry: string) {
  const onCollapse = jest.fn();
  const onMenuSelect = jest.fn();
  const result = render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <TooltipProvider>
        <FilesSidebar onCollapse={onCollapse} onMenuSelect={onMenuSelect} />
      </TooltipProvider>
    </MemoryRouter>,
  );
  return { ...result, onCollapse, onMenuSelect };
}

describe('FilesSidebar', () => {
  it('renders only All files and keeps it selected inside folders', () => {
    const { onMenuSelect } = renderSidebar('/files?folderId=folder-1');

    expect(screen.getByText('sidebar.library')).toBeInTheDocument();
    expect(screen.getAllByRole('link')).toHaveLength(1);
    const allFiles = screen.getByRole('link', {
      name: 'fileManager.allFiles',
    });
    expect(allFiles).toHaveAttribute('href', '/files');
    expect(allFiles).toHaveAttribute('aria-current', 'page');

    fireEvent.click(allFiles);
    expect(onMenuSelect).toHaveBeenCalledTimes(1);
  });

  it('does not mark All files active on the Skills route', () => {
    renderSidebar('/files/skills');

    expect(
      screen.getByRole('link', { name: 'fileManager.allFiles' }),
    ).not.toHaveAttribute('aria-current');
  });

  it('renders a working collapse control in the header', () => {
    const { onCollapse } = renderSidebar('/files');

    fireEvent.click(
      screen.getByRole('button', { name: 'sidebar.collapseSidebar' }),
    );

    expect(onCollapse).toHaveBeenCalledTimes(1);
  });
});
