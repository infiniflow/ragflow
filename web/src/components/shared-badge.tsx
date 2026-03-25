import { useFetchUserInfo } from '@/hooks/use-user-setting-request';
import { PropsWithChildren } from 'react';

export function SharedBadge({ children }: PropsWithChildren) {
  const { data: userInfo } = useFetchUserInfo();

  if (typeof children === 'string' && userInfo.nickname === children) {
    return null;
  }
  
  return <span title={typeof children === 'string' ? children : undefined} className="inline-block max-w-[120px] truncate align-middle bg-bg-card rounded-sm px-1 text-xs">{children}</span>;
}
