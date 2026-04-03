import { Button } from '@/components/ui/button';
import { useNavigateWithFromState } from '@/hooks/route-hook';
import { useListTenant } from '@/hooks/use-user-setting-request';
import { TenantRole } from '@/pages/user-setting/constants';
import { BellRing } from 'lucide-react';
import { useCallback, useMemo } from 'react';

export function BellButton() {
  const { data } = useListTenant();
  const navigate = useNavigateWithFromState();

  const showBell = useMemo(() => {
    return data.some((x) => x.role === TenantRole.Invite);
  }, [data]);

  const handleBellClick = useCallback(() => {
    navigate('/user-setting/team');
  }, [navigate]);

  return showBell ? (
    <Button variant={'ghost'} onClick={handleBellClick}>
      <div className="relative">
        <BellRing className="size-4 " />
        <span className="absolute size-1 rounded -right-1 -top-1 bg-red-600"></span>
      </div>
    </Button>
  ) : null;
}
