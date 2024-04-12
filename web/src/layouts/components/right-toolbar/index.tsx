import { ReactComponent as TranslationIcon } from '@/assets/svg/translation.svg';
import { useTranslate } from '@/hooks/commonHooks';
import { GithubOutlined } from '@ant-design/icons';
import { Dropdown, MenuProps, Space } from 'antd';
import camelCase from 'lodash/camelCase';
import React from 'react';
import User from '../user';

import { LanguageList } from '@/constants/common';
import { useChangeLanguage } from '@/hooks/logicHooks';
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
        <Circle>
          <GithubOutlined onClick={handleGithubCLick} />
        </Circle>
        <Dropdown menu={{ items, onClick: handleItemClick }} placement="bottom">
          <Circle>
            <TranslationIcon />
          </Circle>
        </Dropdown>
        {/* <Circle>
          <MonIcon />
        </Circle> */}
        <User></User>
      </Space>
    </div>
  );
};

export default RightToolBar;
