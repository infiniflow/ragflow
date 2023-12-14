import { onBeforeMount } from 'vue';
import { ConfigProvider } from 'ant-design-vue';

const LOCAL_THEME = 'local_theme';

export const colors: string[] = [
  '#f5222d',
  '#fa541c',
  '#fa8c16',
  '#a0d911',
  '#13c2c2',
  '#1890ff',
  '#722ed1',
  '#eb2f96',
  '#faad14',
  '#52c41a',
];

export const randomTheme = (): string => {
  const i = Math.floor(Math.random() * 10);
  return colors[i];
};

export const load = () => {
  const color = localStorage.getItem(LOCAL_THEME) || '#1890ff';
  return color;
};

export const save = (color: string) => {
  localStorage.setItem(LOCAL_THEME, color);
};

export const apply = (color: string) => {
  ConfigProvider.config({
    theme: {
      primaryColor: color,
    },
  });
  save(color);
};

export const useUserTheme = () => {
  onBeforeMount(() => {
    apply(load());
  });
};
