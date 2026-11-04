import { Routes } from '@/routes';
import { PropsWithChildren } from 'react';
import { Link } from 'react-router';

/**
 * Enterprise slot around the notification bell. The enterprise build replaces
 * this file with the team invitation dialog; open source keeps the bell as a
 * shortcut to the team page.
 */
export function TeamInvitationReminderDialog({ children }: PropsWithChildren) {
  return (
    <Link
      to={`${Routes.UserSetting}${Routes.Team}`}
      className="inline-flex items-center"
    >
      {children}
    </Link>
  );
}
