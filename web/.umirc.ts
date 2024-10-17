import path from 'path';
import { defineConfig } from 'umi';
import { appName } from './src/conf.json';
import routes from './src/routes';

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
  clickToComponent: {},
  history: {
    type: 'browser',
  },
  plugins: ['@react-dev-inspector/umi4-plugin'],
  jsMinifier: 'terser',
  lessLoader: {
    modifyVars: {
      hack: `true; @import "~@/less/index.less";`,
    },
  },
  devtool: 'source-map',
  copy: ['src/conf.json'],
  proxy: {
    '/v1': {
      target: 'http://127.0.0.1:9456/',
      changeOrigin: true,
      ws: true,
      logger: console,
      // pathRewrite: { '^/v1': '/v1' },
    },
  },
  chainWebpack(memo, args) {
    memo.module.rule('markdown').test(/\.md$/).type('asset/source');

    return memo;
  },
});
