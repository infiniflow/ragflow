import { fireEvent, render, screen } from '@testing-library/react';
import React from 'react';

jest.mock('@/components/confirm-delete-dialog', () => ({
  ConfirmDeleteDialog: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}));

jest.mock('@/components/ui/dropdown-menu', () => {
  const DropdownMenu = ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  );
  type TriggerProps = React.PropsWithChildren<
    React.HTMLAttributes<HTMLElement> & { asChild?: boolean }
  >;
  const DropdownMenuTrigger = ({
    children,
    asChild,
    ...props
  }: TriggerProps) =>
    asChild
      ? React.cloneElement(
          children as React.ReactElement<React.HTMLAttributes<HTMLElement>>,
          props,
        )
      : children;
  const DropdownMenuContent = ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  );
  const DropdownMenuItem = ({
    children,
    ...props
  }: React.PropsWithChildren<
    React.ButtonHTMLAttributes<HTMLButtonElement>
  >) => <button {...props}>{children}</button>;

  return {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuTrigger,
  };
});

jest.mock('@/hooks/use-agent-request', () => ({
  useDeleteAgentSession: () => ({
    deleteAgentSession: jest.fn(),
  }),
}));

jest.mock('../hooks/use-explore-url-params', () => ({
  useExploreUrlParams: () => ({
    canvasId: 'canvas-1',
    setSessionId: jest.fn(),
    sessionId: 'session-1',
  }),
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('lucide-react', () => ({
  Trash2: () => <span />,
}));

import { SessionDropdown } from './session-dropdown';

describe('SessionDropdown', () => {
  it('does not let the trigger click select the surrounding session card', () => {
    const onCardClick = jest.fn();

    render(
      <div onClick={onCardClick}>
        <SessionDropdown
          session={{ id: 'session-1', name: 'Session 1' } as never}
        >
          <button data-testid="session-menu-trigger" type="button">
            More
          </button>
        </SessionDropdown>
      </div>,
    );

    fireEvent.click(screen.getByTestId('session-menu-trigger'));

    expect(onCardClick).not.toHaveBeenCalled();
  });
});
