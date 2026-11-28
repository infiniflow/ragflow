import { TooltipProvider } from '@/components/ui/tooltip';
import fileManagerService from '@/services/file-manager-service';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import Files from '..';

jest.mock('@/services/file-manager-service', () => ({
  __esModule: true,
  default: { listFile: jest.fn(), removeFile: jest.fn() },
}));
jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { success: jest.fn() },
}));
jest.mock('react-i18next', () => ({
  ...jest.requireActual('react-i18next'),
  useTranslation: () => ({
    t: (key: string, options?: { count: number }) =>
      options?.count === undefined ? key : `${key} ${options.count}`,
  }),
}));

const files = ['alpha', 'beta'].map((name) => ({
  id: `file-${name}`,
  name: `${name}.pdf`,
  type: 'pdf',
  size: 1,
  kbs_info: [],
}));

function selectFile(name: string) {
  const row = screen.getByRole('row', { name: new RegExp(name) });
  fireEvent.click(within(row).getByRole('checkbox'));
}

it.each(['before typing', 'while search is debouncing'])(
  'deletes only visible files when a file was selected %s on page one',
  async (when) => {
    jest.clearAllMocks();
    jest
      .mocked(fileManagerService.listFile)
      .mockImplementation(async ({ keywords }) => {
        const matches = files.filter(({ name }) =>
          name.includes(keywords || ''),
        );
        return {
          data: { code: 0, data: { files: matches, total: matches.length } },
        };
      });
    jest
      .mocked(fileManagerService.removeFile)
      .mockResolvedValue({ data: { code: 0 } });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <MemoryRouter>
        <QueryClientProvider client={client}>
          <TooltipProvider>
            <Files />
          </TooltipProvider>
        </QueryClientProvider>
      </MemoryRouter>,
    );
    await screen.findByText('alpha.pdf');
    if (when === 'before typing') selectFile('alpha');
    fireEvent.change(screen.getByRole('searchbox'), {
      target: { value: 'beta' },
    });
    expect(
      screen.queryByRole('menu', { name: 'common.bulkOperate' }),
    ).toBeNull();
    if (when === 'while search is debouncing') selectFile('alpha');

    await waitFor(() =>
      expect(fileManagerService.listFile).toHaveBeenCalledWith(
        expect.objectContaining({ keywords: 'beta', page: 1 }),
      ),
    );
    await screen.findByText('beta.pdf');
    expect(screen.queryByText('alpha.pdf')).toBeNull();
    expect(
      screen.queryByRole('menu', { name: 'common.bulkOperate' }),
    ).toBeNull();
    selectFile('beta');
    fireEvent.click(screen.getByRole('menuitem', { name: 'common.delete' }));
    const dialog = screen.getByRole('alertdialog');
    const confirmation = dialog.textContent;
    fireEvent.click(
      within(dialog).getByTestId('confirm-delete-dialog-confirm-btn'),
    );
    await waitFor(() =>
      expect(fileManagerService.removeFile).toHaveBeenCalledWith({
        ids: ['file-beta'],
      }),
    );
    expect(fileManagerService.removeFile).toHaveBeenCalledTimes(1);
    expect(confirmation).toContain('deleteModal.delFilesContent 1');
  },
);
