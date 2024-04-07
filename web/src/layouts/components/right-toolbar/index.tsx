import { ReactComponent as TranslationIcon } from '@/assets/svg/translation.svg';
import { useTranslate } from '@/hooks/commonHooks';
import { GithubOutlined } from '@ant-design/icons';
import { Dropdown, MenuProps, Space } from 'antd';
import React from 'react';
import User from '../user';

import styled from './index.less';

const Circle = ({ children }: React.PropsWithChildren) => {
  return <div className={styled.circle}>{children}</div>;
};

const handleGithubCLick = () => {
  window.open('https://github.com/infiniflow/ragflow', 'target');
};

const RightToolBar = () => {
  const { t, i18n } = useTranslate('common');

  const handleItemClick: MenuProps['onClick'] = ({ key }) => {
    i18n.changeLanguage(key);
  };

  const items: MenuProps['items'] = [
    {
      key: 'en',
      label: <span>{t('english')}</span>,
    },
    { type: 'divider' },
    {
      key: 'zh',
      label: <span>{t('chinese')}</span>,
    },
  ];

  return (
    <div className={styled.toolbarWrapper}>
      <Space wrap size={16}>
        <Circle>
          <GithubOutlined onClick={handleGithubCLick} />
        </Circle>
        <Circle>
          <Dropdown
            menu={{ items, onClick: handleItemClick }}
            placement="bottom"
          >
            <TranslationIcon />
          </Dropdown>
        </Circle>
        {/* <Circle>
          <MoonIcon />
        </Circle> */}
        <User></User>
      </Space>
    </div>
  );
};

export default RightToolBar;
