import { useTranslate } from '@/hooks/commonHooks';
import { DownOutlined, GithubOutlined } from '@ant-design/icons';
import { Dropdown, MenuProps, Space } from 'antd';
import camelCase from 'lodash/camelCase';
import React from 'react';
import User from '../user';

import { LanguageList } from '@/constants/common';
import { useChangeLanguage } from '@/hooks/logicHooks';
import { useSelector } from 'umi';
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

const RightToolBar = () => {
  const { t } = useTranslate('common');
  const changeLanguage = useChangeLanguage();
  const { language = '' } = useSelector((state) => state.settingModel.userInfo);

  const handleItemClick: MenuProps['onClick'] = ({ key }) => {
    changeLanguage(key);
  };

  const items: MenuProps['items'] = LanguageList.map((x) => ({
    key: x,
    label: <span>{t(camelCase(x))}</span>,
  })).reduce<MenuProps['items']>((pre, cur) => {
    return [...pre!, { type: 'divider' }, cur];
  }, []);

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
        {/* <Circle>
          <MonIcon />
        </Circle> */}
        <User></User>
      </Space>
    </div>
  );
};

export default RightToolBar;
