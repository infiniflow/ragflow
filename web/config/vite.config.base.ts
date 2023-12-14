import { URL, fileURLToPath } from 'node:url';
import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import vueJsx from '@vitejs/plugin-vue-jsx';
import AutoImport from 'unplugin-auto-import/vite';
import Components from 'unplugin-vue-components/vite';
import { AntDesignVueResolver } from 'unplugin-vue-components/resolvers';
import UnoCSS from 'unocss/vite';

export default defineConfig({
  plugins: [
    vue(),
    vueJsx(),
    AutoImport({
      eslintrc: {
        enabled: true,
      },
      include: [
        /\.[tj]sx?$/, // .ts, .tsx, .js, .jsx
        /\.vue$/, /\.vue\?vue/, // .vue
      ],
      imports: [
        'vue',
        'vue-router',
        '@vueuse/core',
        {
          'axios': [
            ['default', 'axios'],
          ],
          '@vueuse/integrations/useAxios': [
            'useAxios',
          ],
          'ant-design-vue': ['message'],
          'nprogress': [
            ['default', 'NProgress'],
          ],
          'mitt': [
            ['default', 'mitt'],
          ],
          'mockjs': [
            ['default', 'Mock'],
          ],
        },
        {
          from: 'vue-router',
          imports: ['LocationQueryRaw', 'Router', 'RouteLocationNormalized', 'RouteRecordRaw', 'RouteRecordNormalized', 'RouteLocationRaw'],
          type: true,
        },
        {
          from: 'mitt',
          imports: ['Handler'],
          type: true,
        },
        {
          from: 'axios',
          imports: ['RawAxiosRequestConfig'],
          type: true,
        },
      ],
      dirs: [
        './src/utils',
        './src/components',
        './src/hooks',
        './src/store',
      ],
    }),
    Components({
      dts: true,
      resolvers: [
        AntDesignVueResolver({
          importStyle: false,
          resolveIcons: true,
        }),
      ],
    }),
    UnoCSS(),
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('../src', import.meta.url)),
    },
  },
  define: {
    'process.env': {},
  },
  css: {
    preprocessorOptions: {
      less: {
        // DO NOT REMOVE THIS LINE
        javascriptEnabled: true,
        modifyVars: {
          // hack: `true; @import 'ant-design-vue/dist/antd.variable.less'`,
          // '@primary-color': '#eb2f96', // 全局主色
        },
      },
    },
  },
  optimizeDeps: {
    include: [
      '@ant-design/icons-vue',
      'ant-design-vue',
      '@ant-design-vue/pro-layout',
      'ant-design-vue/es',
      'vue',
      'vue-router',
    ],
  },
});
