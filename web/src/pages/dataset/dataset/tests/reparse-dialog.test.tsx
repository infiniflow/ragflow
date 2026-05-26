jest.mock('react-i18next', () => {
  const t = (key: string) => key;
  return { useTranslation: () => ({ t }) };
});

jest.mock('@/components/confirm-delete-dialog', () => ({
  ConfirmDeleteDialog: ({
    children,
    hidden,
  }: {
    children?: React.ReactNode;
    hidden?: boolean;
  }) =>
    hidden ? <>{children}</> : <div data-testid="reparse-dialog-visible" />,
}));

jest.mock('@/components/dynamic-form', () => ({
  DynamicForm: { Root: () => null },
  FormFieldType: { Checkbox: 'checkbox' },
}));

jest.mock('@/components/ui/checkbox', () => ({
  Checkbox: () => null,
}));

import { act, render } from '@testing-library/react';
import * as React from 'react';
import { ReparseDialog } from '../reparse-dialog';

function renderDialog(
  overrides: Partial<React.ComponentProps<typeof ReparseDialog>> = {},
) {
  const handleOperationIconClick = jest.fn();
  const hideModal = jest.fn();
  const props = {
    visible: true,
    hidden: true,
    chunk_num: 0,
    enable_metadata: false,
    handleOperationIconClick,
    hideModal,
    ...overrides,
  } as React.ComponentProps<typeof ReparseDialog>;
  const utils = render(<ReparseDialog {...props} />);
  return { ...utils, handleOperationIconClick, hideModal };
}

describe('ReparseDialog auto-fire when hidden', () => {
  it('fires handleOperationIconClick exactly once on a normal mount', () => {
    const { handleOperationIconClick } = renderDialog();
    expect(handleOperationIconClick).toHaveBeenCalledTimes(1);
  });

  it('still fires exactly once under StrictMode (effect double-invoke)', () => {
    const handleOperationIconClick = jest.fn();
    const hideModal = jest.fn();
    act(() => {
      render(
        <React.StrictMode>
          <ReparseDialog
            visible
            hidden
            chunk_num={0}
            enable_metadata={false}
            handleOperationIconClick={handleOperationIconClick}
            hideModal={hideModal}
          />
        </React.StrictMode>,
      );
    });
    expect(handleOperationIconClick).toHaveBeenCalledTimes(1);
  });

  it('does not auto-fire when hidden is false', () => {
    const { handleOperationIconClick } = renderDialog({ hidden: false });
    expect(handleOperationIconClick).not.toHaveBeenCalled();
  });
});
