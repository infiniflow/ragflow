import { ReactComponent as MoonIcon } from '@/assets/svg/moon.svg';
import { ReactComponent as TranslationIcon } from '@/assets/svg/translation.svg';
import { BellOutlined, GithubOutlined } from '@ant-design/icons';
import { Space } from 'antd';
import React from 'react';
import User from '../user';
import styled from './index.less';

const Circle = ({ children }: React.PropsWithChildren) => {
  return <div className={styled.circle}>{children}</div>;
};

const handleGithubCLick = () => {
  window.open('https://github.com/infiniflow/infinity', 'target');
};

const RightToolBar = () => {
  return (
    <div className={styled.toolbarWrapper}>
      <Space wrap size={16}>
        <Circle>
          <GithubOutlined onClick={handleGithubCLick} />
        </Circle>
        <Circle>
          <TranslationIcon />
        </Circle>
        <Circle>
          <BellOutlined />
        </Circle>
        <Circle>
          <MoonIcon />
        </Circle>
        <User></User>
      </Space>
    </div>
  );
};

export default RightToolBar;
