import { useFetchUserInfo } from '@/hooks/user-setting-hooks';
import { PropsWithChildren } from 'react';

export function SharedBadge({ children }: PropsWithChildren) {
  const { data: userInfo } = useFetchUserInfo();

  if (typeof children === 'string' && userInfo.nickname === children) {
    return null;
  }

  return <span className="bg-bg-card rounded-sm px-1 text-xs">{children}</span>;
}
