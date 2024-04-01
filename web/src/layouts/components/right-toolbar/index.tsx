import { GithubOutlined } from '@ant-design/icons';
import { Space } from 'antd';
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
  return (
    <div className={styled.toolbarWrapper}>
      <Space wrap size={16}>
        <Circle>
          <GithubOutlined onClick={handleGithubCLick} />
        </Circle>
        {/* <Circle>
          <TranslationIcon />
        </Circle>
        <Circle>
          <MoonIcon />
        </Circle> */}
        <User></User>
      </Space>
    </div>
  );
};

export default RightToolBar;
