import { defineConfig } from 'umi';
import routes from './src/routes';

export default defineConfig({
  outputPath: 'dist',
  // alias: { '@': './src' },
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
  plugins: ['@react-dev-inspector/umi4-plugin', '@umijs/plugins/dist/dva'],
  dva: {},

  lessLoader: {
    modifyVars: {
      hack: `true; @import "~@/less/index.less";`,
    },
  },
  devtool: 'source-map',
  proxy: {
    '/v1': {
      target: 'http://192.168.200.233:9380/',
      changeOrigin: true,
      // pathRewrite: { '^/v1': '/v1' },
    },
  },
});
