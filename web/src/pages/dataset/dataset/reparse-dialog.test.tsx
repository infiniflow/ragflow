import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { ReparseDialog } from './reparse-dialog';

// The dialog's field-building effect depends on `t`, so the mock must return
// a stable reference — a fresh closure per render would loop the effect.
const mockT = (key: string, params?: Record<string, unknown>) => {
  if (key === 'knowledgeDetails.redo') {
    return `clear existing ${params?.chunkNum} chunks`;
  }
  return key;
};

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: mockT }),
}));

const handleOperationIconClick = jest.fn();
const hideModal = jest.fn();

function renderDialog(props: Record<string, unknown> = {}) {
  return render(
    <ReparseDialog
      chunk_num={10}
      handleOperationIconClick={handleOperationIconClick}
      visible={true}
      hideModal={hideModal}
      {...props}
    />,
  );
}

function confirm() {
  fireEvent.click(screen.getByRole('button', { name: 'common.confirm' }));
}

describe('ReparseDialog', () => {
  beforeEach(() => {
    handleOperationIconClick.mockClear();
    hideModal.mockClear();
  });

  it('offers the keep-chunks choice and submits it on Python', async () => {
    renderDialog();

    const clearOption = screen.getByRole('checkbox');
    expect(clearOption).toBeChecked();

    fireEvent.click(clearOption);
    expect(clearOption).not.toBeChecked();

    confirm();
    await waitFor(() =>
      expect(handleOperationIconClick).toHaveBeenCalledWith({
        delete: false,
        apply_kb: false,
      }),
    );
  });

  it('omits the choice and always clears chunks when asked to', async () => {
    renderDialog({ alwaysClearChunks: true });

    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();

    confirm();
    await waitFor(() =>
      expect(handleOperationIconClick).toHaveBeenCalledWith({
        delete: true,
        apply_kb: false,
      }),
    );
  });

  it('keeps the auto-metadata choice when chunks are always cleared', async () => {
    renderDialog({ alwaysClearChunks: true, enable_metadata: true });

    const metadataOption = screen.getByRole('checkbox');
    expect(metadataOption).not.toBeChecked();

    fireEvent.click(metadataOption);
    confirm();
    await waitFor(() =>
      expect(handleOperationIconClick).toHaveBeenCalledWith({
        delete: true,
        apply_kb: true,
      }),
    );
  });

  it('submits no forced delete for a chunkless document', async () => {
    renderDialog({ chunk_num: 0, alwaysClearChunks: true });

    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();

    confirm();
    await waitFor(() =>
      expect(handleOperationIconClick).toHaveBeenCalledWith({
        delete: false,
        apply_kb: false,
      }),
    );
  });
});
