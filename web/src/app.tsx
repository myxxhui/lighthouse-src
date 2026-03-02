import './global.less';
import React from 'react';
import { ConfigProvider } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import { theme as antdTheme } from 'antd';
import { useAppStore } from '@/store';

const ThemeWrapper: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const theme = useAppStore(s => s.theme);
  const algorithm = theme === 'dark' ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm;
  return (
    <ConfigProvider locale={zhCN} theme={{ algorithm }}>
      {children}
    </ConfigProvider>
  );
};

export const rootContainer = (container: React.ReactNode) => {
  return <ThemeWrapper>{container}</ThemeWrapper>;
};
