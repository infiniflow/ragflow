import { ConfigProvider } from 'antd';
import React, { ReactNode } from 'react';

export function rootContainer(container: ReactNode) {
  return React.createElement(
    ConfigProvider,
    {
      theme: {
        token: {
          fontFamily: 'Inter',
        },
      },
    },
    container,
  );
}
