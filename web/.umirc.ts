import path from 'path';
import { defineConfig } from 'umi';
import routes from './src/routes';

const cMapsDir = path.join(
  path.dirname(require.resolve('pdfjs-dist/package.json')),
  'cmaps',
);
const standardFontsDir = path.join(
  path.dirname(require.resolve('pdfjs-dist/package.json')),
  'standard_fonts',
);

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
  history: {
    type: 'browser',
  },
  plugins: ['@react-dev-inspector/umi4-plugin', '@umijs/plugins/dist/dva'],
  dva: {},
  lessLoader: {
    modifyVars: {
      hack: `true; @import "~@/less/variable.less";`,
    },
  },
  proxy: {
    '/v1': {
      target: 'http://123.60.95.134:9380/',
      changeOrigin: true,
      // pathRewrite: { '^/v1': '/v1' },
    },
  },
  copy: [
    { from: cMapsDir, to: 'cmaps/' },
    { from: standardFontsDir, to: 'standard_fonts/' },
  ],
  chainWebpack(memo, args) {
    console.info(memo);
  },
});
