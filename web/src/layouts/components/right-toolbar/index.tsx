import { useTranslate } from '@/hooks/common-hooks';
import { DownOutlined, GithubOutlined } from '@ant-design/icons';
import { Dropdown, MenuProps, Space } from 'antd';
import camelCase from 'lodash/camelCase';
import React, { useCallback, useMemo } from 'react';
import User from '../user';

import { useTheme } from '@/components/theme-provider';
import { LanguageList, LanguageMap, ThemeEnum } from '@/constants/common';
import { useChangeLanguage } from '@/hooks/logic-hooks';
import {
  useFetchUserInfo,
  useListTenant,
} from '@/hooks/use-user-setting-request';
import { TenantRole } from '@/pages/user-setting/constants';
import { BellRing, CircleHelp, MoonIcon, SunIcon } from 'lucide-react';
import { useNavigate } from 'react-router';
import styled from './index.less';

const Circle = ({ children, ...restProps }: React.PropsWithChildren) => {
  return (
    <div {...restProps} className={styled.circle}>
      {children}
    </div>
  );
};

const handleGithubCLick = () => {
  window.open('https://github.com/infiniflow/ragflow', 'target');
};

const handleDocHelpCLick = () => {
  window.open('https://ragflow.io/docs/dev/category/guides', 'target');
};

const RightToolBar = () => {
  const { t } = useTranslate('common');
  const changeLanguage = useChangeLanguage();
  const { setTheme, theme } = useTheme();
  const navigate = useNavigate();

  const {
    data: { language = 'English' },
  } = useFetchUserInfo();

  const handleItemClick: MenuProps['onClick'] = ({ key }) => {
    changeLanguage(key);
  };

  const { data } = useListTenant();

  const showBell = useMemo(() => {
    return data.some((x) => x.role === TenantRole.Invite);
  }, [data]);

  const items: MenuProps['items'] = LanguageList.map((x) => ({
    key: x,
    label: <span>{LanguageMap[x as keyof typeof LanguageMap]}</span>,
  })).reduce<MenuProps['items']>((pre, cur) => {
    return [...pre!, { type: 'divider' }, cur];
  }, []);

  const onMoonClick = React.useCallback(() => {
    setTheme(ThemeEnum.Light);
  }, [setTheme]);
  const onSunClick = React.useCallback(() => {
    setTheme(ThemeEnum.Dark);
  }, [setTheme]);

  const handleBellClick = useCallback(() => {
    navigate('/user-setting/team');
  }, [navigate]);

  return (
    <div className={styled.toolbarWrapper}>
      <Space wrap size={16}>
        <Dropdown menu={{ items, onClick: handleItemClick }} placement="bottom">
          <Space className={styled.language}>
            <b>{t(camelCase(language))}</b>
            <DownOutlined />
          </Space>
        </Dropdown>
        <Circle>
          <GithubOutlined onClick={handleGithubCLick} />
        </Circle>
        <Circle>
          <CircleHelp className="size-4" onClick={handleDocHelpCLick} />
        </Circle>
        <Circle>
          {theme === 'dark' ? (
            <MoonIcon onClick={onMoonClick} size={20} />
          ) : (
            <SunIcon onClick={onSunClick} size={20} />
          )}
        </Circle>
        {showBell && (
          <Circle>
            <div className="relative" onClick={handleBellClick}>
              <BellRing className="size-4 " />
              <span className="absolute size-1 rounded -right-1 -top-1 bg-red-600"></span>
            </div>
          </Circle>
        )}
        <User></User>
      </Space>
    </div>
  );
};

export default RightToolBar;
