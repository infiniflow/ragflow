import type { Meta, StoryObj } from '@storybook/react-webpack5';

import { fn } from 'storybook/test';

import { ButtonLoading } from '@/components/ui/button';

// More on how to set up stories at: https://storybook.js.org/docs/writing-stories#default-export
const meta = {
  title: 'Example/ButtonLoading',
  component: ButtonLoading,
  parameters: {
    // Optional parameter to center the component in the Canvas. More info: https://storybook.js.org/docs/configure/story-layout
    layout: 'centered',
    docs: {
      description: {
        component: `
## Component Description

ButtonLoading is a button component with a loading state and supports displaying loading animation.        `,
      },
    },
  },
  // This component will have an automatically generated Autodocs entry: https://storybook.js.org/docs/writing-docs/autodocs
  tags: ['autodocs'],
  // More on argTypes: https://storybook.js.org/docs/api/argtypes
  argTypes: {
    loading: { control: 'boolean' },
  },
  // Use `fn` to spy on the onClick arg, which will appear in the actions panel once invoked: https://storybook.js.org/docs/essentials/actions#action-args
  args: { onClick: fn() },
} satisfies Meta<typeof ButtonLoading>;

export default meta;
type Story = StoryObj<typeof meta>;

// More on writing stories with args: https://storybook.js.org/docs/writing-stories/args
export const WithLoading: Story = {
  args: {
    loading: true,
    children: 'Button',
  },
  parameters: {
    docs: {
      description: {
        story: `
### Usage Examples

\`\`\`tsx
import { ButtonLoading } from '@/components/ui/button';

<ButtonLoading loading={true}>
  Loading Button
</ButtonLoading>
\`\`\`
        `,
      },
    },
  },
  tags: ['!dev'],
};
