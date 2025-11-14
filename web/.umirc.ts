import path from 'path';
import TerserPlugin from 'terser-webpack-plugin';
import { defineConfig } from 'umi';
import { appName } from './src/conf.json';
import routes from './src/routes';
const ESLintPlugin = require('eslint-webpack-plugin');

export default defineConfig({
  title: appName,
  outputPath: 'dist',
  alias: { '@parent': path.resolve(__dirname, '../') },
  npmClient: 'npm',
  base: '/',
  routes,
  publicPath: '/',
  esbuildMinifyIIFE: true,
  icons: {},
  hash: true,
  favicons: ['/logo.svg'],
  headScripts: [{ src: '/iconfont.js', defer: true }],
  clickToComponent: {},
  history: {
    type: 'browser',
  },
  plugins: [
    '@react-dev-inspector/umi4-plugin',
    '@umijs/plugins/dist/tailwindcss',
  ],
  jsMinifier: 'none', // Fixed the issue that the page displayed an error after packaging lexical with terser
  lessLoader: {
    modifyVars: {
      hack: `true; @import "~@/less/index.less";`,
    },
  },
  devtool: 'source-map',
  copy: [
    { from: 'src/conf.json', to: 'dist/conf.json' },
    { from: 'node_modules/monaco-editor/min/vs/', to: 'dist/vs/' },
  ],
  proxy: [
    {
      context: ['/api/v1/admin'],
      target: 'http://127.0.0.1:9381/',
      changeOrigin: true,
      ws: true,
      logger: console,
    },
    {
      context: ['/api', '/v1'],
      target: 'http://127.0.0.1:9380/',
      changeOrigin: true,
      ws: true,
      logger: console,
      // pathRewrite: { '^/v1': '/v1' },
    },
  ],

  chainWebpack(memo, args) {
    memo.module.rule('markdown').test(/\.md$/).type('asset/source');

    memo.optimization.minimizer('terser').use(TerserPlugin); // Fixed the issue that the page displayed an error after packaging lexical with terser

    // memo.plugin('eslint').use(ESLintPlugin, [
    //   {
    //     extensions: ['js', 'ts', 'tsx'],
    //     failOnError: true,
    //     exclude: ['**/node_modules/**', '**/mfsu**', '**/mfsu-virtual-entry**'],
    //     files: ['src/**/*.{js,ts,tsx}'],
    //   },
    // ]);

    return memo;
  },
  tailwindcss: {},
});
