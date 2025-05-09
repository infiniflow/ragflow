import { Space } from 'antd';
import React, { useCallback, useMemo } from 'react';
import { useNavigate } from 'umi';

import User from '../user';

import { useListTenant } from '@/hooks/user-setting-hooks';
import { TenantRole } from '@/pages/user-setting/constants';

import { BellRing } from 'lucide-react';

import styled from './index.less';

/** Simple circle wrapper for icons */
const Circle = ({
  children,
  ...restProps
}: React.PropsWithChildren<React.HTMLAttributes<HTMLDivElement>>) => {
  return (
    <div {...restProps} className={styled.circle}>
      {children}
    </div>
  );
};

const RightToolBar = () => {
  const navigate = useNavigate();
  const { data } = useListTenant();

  /** show bell when current user has pending team invites */
  const showBell = useMemo(
    () => data.some((x) => x.role === TenantRole.Invite),
    [data],
  );

  const handleBellClick = useCallback(() => {
    navigate('/user-setting/team');
  }, [navigate]);

  return (
    <div className={styled.toolbarWrapper}>
      <Space wrap size={16}>
        {showBell && (
          <Circle>
            <div className="relative cursor-pointer" onClick={handleBellClick}>
              <BellRing className="size-4" />
              <span className="absolute size-1 rounded -right-1 -top-1 bg-red-600"></span>
            </div>
          </Circle>
        )}
        {/* <User /> */}
        {/* User profile temporarily disabled */}
        <div style={{ pointerEvents: 'none', opacity: 0.4 }}>
          <User />
        </div>
      </Space>
    </div>
  );
};

export default RightToolBar;
