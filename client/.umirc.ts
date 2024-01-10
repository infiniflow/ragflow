import { defineConfig } from "umi";
import routes from './routes'

export default defineConfig({
  outputPath: 'dist',
  // alias: { '@': './src' },
  routes,
  npmClient: 'npm',
  base: '/',
  publicPath: '/client/dist/',
  hash: true,
  history: {
    type: 'hash',
  },
  plugins: ['@umijs/plugins/dist/dva'],
  dva: {}
});

