import react from '@vitejs/plugin-react';
import path from 'path';
import { defineConfig, loadEnv } from 'vite';
import { createHtmlPlugin } from 'vite-plugin-html';
import { viteStaticCopy } from 'vite-plugin-static-copy';
import { appName } from './src/conf.json';

// https://vitejs.dev/config/
export default defineConfig(({ mode, command }) => {
  const env = loadEnv(mode, process.cwd(), '');

  return {
    plugins: [
      react(),
      viteStaticCopy({
        targets: [
          {
            src: 'src/conf.json',
            dest: './',
          },
          {
            src: 'node_modules/monaco-editor/min/vs/',
            dest: './',
          },
        ],
      }),
      createHtmlPlugin({
        inject: {
          data: {
            title: appName,
          },
        },
      }),
      // svgLoader({
      //   svgo: true, // 启用SVGO压缩
      //   svgoConfig: {
      //     plugins: [
      //       { name: 'removeViewBox', active:false }, // 保留viewBox属性
      //       { name: 'removeComments', active: true }, // 移除注释
      //       { name: 'removeEmptyAttrs', active: true }, // 移除空属性
      //       { name: 'removeHiddenElems', active: true }, // 移除隐藏元素
      //       { name: 'removeMetadata', active: true }, // 移除元数据
      //       { name: 'removeUselessDefs', active: true }, // 移除无用定义
      //       { name: 'removeXMLProcInst', active: true }, // 移除XML处理指令
      //       { name: 'removeTitle', active: true }, // 移除标题
      //       { name: 'removeDesc', active: true }, // 移除描述
      //       { name: 'removeDimensions', active: true }, // 移除宽高属性
      //       { name: 'removeStyleElement', active: true }, // 移除style标签
      //       { name: 'removeScriptElement', active: true }, // 移除script标签
      //       { name: 'removeOffCanvasPaths', active: true }, // 移除离屏路径
      //       { name: 'removeAttrs', params: { attrs: ['data-name'] } }, // 移除自定义属性
      //     ],
      //   },
      // }),
    ],
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
        '@parent': path.resolve(__dirname, '../'),
      },
    },
    css: {
      modules: {
        localsConvention: 'camelCase',
      },
      postcss: './postcss.config.js',
      preprocessorOptions: {
        less: {
          javascriptEnabled: true,
          additionalData: `
            @import "@/less/variable.less";
            @import "@/less/mixins.less";
          `,
          modifyVars: {
            hack: `true; @import "@/less/index.less";`,
          },
        },
      },
    },
    server: {
      port: 9222,
      proxy: {
        '/api/v1/admin': {
          target: 'http://127.0.0.1:9381/',
          changeOrigin: true,
          ws: true,
        },
        '/api': {
          // target: 'http://127.0.0.1:9380/',
          target: 'http://192.168.1.24:9380/',
          changeOrigin: true,
          ws: true,
        },
        '/v1': {
          // target: 'http://127.0.0.1:9380/',
          target: 'http://192.168.1.24:9380/',
          changeOrigin: true,
          ws: true,
        },
      },
    },
    define: {
      'process.env.UMI_APP_RAGFLOW_ENTERPRISE': JSON.stringify(
        env.UMI_APP_RAGFLOW_ENTERPRISE,
      ),
    },
    assetsInclude: ['**/*.md'],
    // base: env.VITE_BASE_URL ,
    base: process.env.NODE_ENV === 'production' ? '/v3/' : '/',
    publicDir: 'public',
    build: {
      outDir: 'dist',
      assetsDir: 'assets',
      assetsInlineLimit: 4096,
      experimentalMinChunkSize: 30 * 1024,
      rollupOptions: {
        output: {
          // manualChunks(id) {
          //   if (id.includes('/components/')) {
          //     return 'components';
          //   }
          //   if (id.includes('jsoneditor')) {
          //     return 'jsoneditor';
          //   }
          //   if (id.includes('react-dom')) {
          //     return 'react-dom';
          //   }
          //   if (id.includes('react-router')) {
          //     return 'react-router';
          //   }
          //   if (id.includes('one-light')) {
          //     return 'one-light';
          //   }
          //   if (id.includes('txt-preview')) {
          //     return 'txt-preview';
          //   }
          //   if (id.includes('node_modules')) {
          //     return 'node_modules';
          //   }
          //   if (id.includes('/utils/')) return 'utils';
          //   if (id.includes('/components/')) return 'components';

          // },
          // manualChunks: {
          //   // 将大型库单独打包
          //   vendor: ['react', 'react-dom', 'react-router'],
          //   ui: ['antd', '@ant-design/icons'],
          //   utils: ['lodash', 'moment'],
          // },
          chunkFileNames: 'chunk/js/[name]-[hash].js',
          entryFileNames: 'entry/js/[name]-[hash].js',
          assetFileNames: 'assets/[ext]/[name]-[hash].[ext]',
        },
        plugins: [],
      },
      // minify: 'esbuild', // 使用 esbuild 进行代码压缩
      // esbuildOptions: {
      //   minify: true,
      //   drop: ['console', 'debugger'], // 移除 console.log 和 debugger
      // },
      minify: 'terser',
      terserOptions: {
        compress: {
          drop_console: true, // 删除 console 语句
          drop_debugger: true, // 删除 debugger 语句
          pure_funcs: ['console.log'], // 删除指定函数调用
        },
        mangle: {
          properties: {
            regex: /^_/,
          },
        },
        format: {
          comments: false, // 删除注释
        },
      },
      sourcemap: true,
    },
    esbuild: {
      tsconfigRaw: {
        compilerOptions: {
          strict: false,
          noImplicitAny: false,
          skipLibCheck: true,
        },
      },
    },
  };
});
