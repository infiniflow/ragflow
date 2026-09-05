import { useFetchUserInfo } from '@/hooks/use-user-setting-request';
import { PropsWithChildren, Suspense } from 'react';
import { Navigate, useLocation } from 'react-router';
import {
  canAccessUserSettingsPath,
  PROFILE_SETTINGS_PATH,
} from './settings-access-policy';

export function SettingsAccessGuard({
  children,
  adminOnly = false,
}: PropsWithChildren<{ adminOnly?: boolean }>) {
  const { data: userInfo } = useFetchUserInfo();
  const location = useLocation();
  const userIsLoaded = Boolean(userInfo?.id);
  const fallback = (
    <div
      aria-busy="true"
      data-testid="settings-access-loading"
      className="size-full"
    />
  );

  if (!userIsLoaded) {
    return fallback;
  }

  if (
    !canAccessUserSettingsPath(
      location.pathname,
      Boolean(userInfo?.is_superuser),
      adminOnly,
    )
  ) {
    return <Navigate to={PROFILE_SETTINGS_PATH} replace />;
  }

  return <Suspense fallback={fallback}>{children}</Suspense>;
}
