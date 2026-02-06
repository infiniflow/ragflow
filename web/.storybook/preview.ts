import '@/locales/config';
import type { Preview } from '@storybook/react-webpack5';
import { createElement } from 'react';
import '../public/iconfont.js';
import { TooltipProvider } from '../src/components/ui/tooltip';

import '../tailwind.css';

const preview: Preview = {
  parameters: {
    controls: {
      matchers: {
        color: /(background|color)$/i,
        date: /Date$/i,
      },
    },
  },
  decorators: [
    (Story) => createElement(TooltipProvider, null, createElement(Story)),
  ],
};

export default preview;
