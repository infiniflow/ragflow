import { App, ConfigProvider } from 'antd';
import { ReactNode } from 'react';

export function rootContainer(container: ReactNode) {
  return (
    <ConfigProvider
      theme={{
        token: {
          fontFamily: 'Inter',
        },
      }}
    >
      <App> {container}</App>
    </ConfigProvider>
  );
}
