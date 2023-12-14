import { mergeConfig } from 'vite';
import baseConfig from './vite.config.base';
import configCompressPlugin from './plugin/compress';

export default mergeConfig(
  {
    mode: 'production',
    base: '/docgpt',
    plugins: [
      configCompressPlugin('gzip'),
    ],
    build: {
      rollupOptions: {
        output: {
          manualChunks: {
            ant: ['ant-design-vue', '@ant-design-vue/pro-layout'],
            vue: ['vue', 'vue-router', 'pinia', '@vueuse/core'],
          },
        },
      },
      chunkSizeWarningLimit: 2000,
    },
  },
  baseConfig,
);
