import { defineConfig, presetAttributify, presetIcons, presetUno } from 'unocss';

const presetIconsConfig = {
  extraProperties: {
    display: 'inline-block',
  },
};
export default defineConfig({
  content: {
    pipeline: {
      exclude: ['node_modules', '.git', '.github', '.husky', '.vscode', 'build', 'config', 'dist', 'public'],
    },
  },
  presets: [
    presetUno({ important: true }),
    presetAttributify(),
    presetIcons(presetIconsConfig),
  ],
});
