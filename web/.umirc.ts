import { defineConfig } from "umi";
import routes from './routes'

export default defineConfig({
  outputPath: 'dist',
  // alias: { '@': './src' },
  routes,
  npmClient: 'npm',
  base: '/',
  publicPath: '/web/dist/',
  esbuildMinifyIIFE: true,
  icons: {

  },
  hash: true,
  history: {
    type: 'browser',
  },
  plugins: ['@umijs/plugins/dist/dva'],
  dva: {},
  // proxy: {
  //   '/v1': {
  //     'target': 'http://54.80.112.79:9380/',
  //     'changeOrigin': true,
  //     'pathRewrite': { '^/v1': '/v1' },
  //   },
  // },
});

