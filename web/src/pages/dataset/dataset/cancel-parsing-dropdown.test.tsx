import { fireEvent, render, screen, waitFor } from '@testing-library/react';

// NOTE: this suite uses jest.mock, so it is transformed by esbuild-jest's
// babel pipeline, which does not strip TypeScript. Imported bindings must not
// appear in type annotations and factory-referenced outer variables must keep
// the `mock` prefix (babel-plugin-jest-hoist restriction).
const mockStopAllDocuments = jest.fn();
let mockStatusCounts: Record<string, number> = {};

jest.mock('react-router', () => ({
  useParams: () => ({ id: 'kb-1' }),
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: { count?: number }) =>
      options?.count === undefined ? key : `${key}:${options.count}`,
  }),
}));

jest.mock('@/hooks/use-document-request', () => ({
  useGetDocumentFilter: () => ({ statusCounts: mockStatusCounts }),
  useStopAllDocuments: () => ({ stopAllDocuments: mockStopAllDocuments }),
}));

import { TooltipProvider } from '@/components/ui/tooltip';
import { CancelParsingDropdown } from './cancel-parsing-dropdown';

beforeAll(() => {
  // jsdom lacks APIs Radix Popper/Menu rely on.
  global.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  if (!Element.prototype.hasPointerCapture) {
    Element.prototype.hasPointerCapture = () => false;
  }
  if (!Element.prototype.scrollIntoView) {
    Element.prototype.scrollIntoView = () => {};
  }
});

beforeEach(() => {
  mockStopAllDocuments.mockReset();
  mockStopAllDocuments.mockResolvedValue({ code: 0 });
  mockStatusCounts = {};
});

function renderDropdown() {
  return render(
    <TooltipProvider>
      <CancelParsingDropdown />
    </TooltipProvider>,
  );
}

function openMenu() {
  // Radix DropdownMenuTrigger toggles on pointerdown and on ArrowDown/Enter.
  // jsdom pointer events do not reliably drive Radix, so open via keyboard.
  fireEvent.keyDown(screen.getByRole('button'), { key: 'ArrowDown' });
}

describe('CancelParsingDropdown', () => {
  it('renders nothing when no parsing or pending files exist', () => {
    mockStatusCounts = { COMPLETED: 5 };
    const { container } = renderDropdown();
    expect(container.querySelector('button')).toBeNull();
  });

  it('renders the trigger when only pending files exist', () => {
    mockStatusCounts = { SCHEDULED: 7 };
    renderDropdown();
    expect(screen.getByRole('button')).toBeInTheDocument();
  });

  it('shows per-status counts in the dropdown', async () => {
    mockStatusCounts = { RUNNING: 297, SCHEDULED: 527 };
    renderDropdown();
    openMenu();
    expect(
      await screen.findByText('knowledgeDetails.filesParsing:297'),
    ).toBeInTheDocument();
    expect(
      screen.getByText('knowledgeDetails.filesPending:527'),
    ).toBeInTheDocument();
  });

  it('disables a row whose status count is zero', async () => {
    mockStatusCounts = { RUNNING: 3 };
    renderDropdown();
    openMenu();
    const pendingItem = await screen.findByText(
      'knowledgeDetails.filesPending:0',
    );
    expect(pendingItem.closest('[role="menuitem"]')).toHaveAttribute(
      'aria-disabled',
      'true',
    );
  });

  it('asks for confirmation and cancels RUNNING tasks on ok', async () => {
    mockStatusCounts = { RUNNING: 297, SCHEDULED: 527 };
    renderDropdown();
    openMenu();
    fireEvent.click(
      await screen.findByText('knowledgeDetails.filesParsing:297'),
    );

    expect(
      await screen.findByText('knowledgeDetails.cancelParsingConfirm:297'),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('confirm-delete-dialog-confirm-btn'));
    await waitFor(() =>
      expect(mockStopAllDocuments).toHaveBeenCalledWith({
        datasetId: 'kb-1',
        runStatuses: ['RUNNING'],
      }),
    );
  });

  it('asks for confirmation and cancels SCHEDULED tasks on ok', async () => {
    mockStatusCounts = { RUNNING: 297, SCHEDULED: 527 };
    renderDropdown();
    openMenu();
    fireEvent.click(
      await screen.findByText('knowledgeDetails.filesPending:527'),
    );

    expect(
      await screen.findByText('knowledgeDetails.cancelPendingConfirm:527'),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('confirm-delete-dialog-confirm-btn'));
    await waitFor(() =>
      expect(mockStopAllDocuments).toHaveBeenCalledWith({
        datasetId: 'kb-1',
        runStatuses: ['SCHEDULED'],
      }),
    );
  });

  it('does not call the api when the confirm dialog is dismissed', async () => {
    mockStatusCounts = { RUNNING: 3 };
    renderDropdown();
    openMenu();
    fireEvent.click(await screen.findByText('knowledgeDetails.filesParsing:3'));
    fireEvent.click(screen.getByTestId('confirm-delete-dialog-cancel-btn'));
    expect(mockStopAllDocuments).not.toHaveBeenCalled();
  });
});
