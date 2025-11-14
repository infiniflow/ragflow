import type { Meta, StoryObj } from '@storybook/react-webpack5';

import { SkeletonCard } from '@/components/skeleton-card';

// More on how to set up stories at: https://storybook.js.org/docs/writing-stories#default-export
const meta = {
  title: 'Example/SkeletonCard',
  component: SkeletonCard,
  parameters: {
    // Optional parameter to center the component in the Canvas. More info: https://storybook.js.org/docs/configure/story-layout
    layout: 'centered',
    docs: {
      description: {
        component: `
## SkeletonCard Component

SkeletonCard is a loading placeholder component that displays skeleton lines while content is being loaded. It provides a consistent loading experience with animated placeholders.

### Import Path
\`\`\`typescript
import { SkeletonCard } from '@/components/skeleton-card';
\`\`\`

### Basic Usage
\`\`\`tsx
import { SkeletonCard } from '@/components/skeleton-card';

function MyComponent() {
  return (
    <SkeletonCard className="w-64" />
  );
}
\`\`\`

### Features
- Displays animated skeleton loading placeholders
- Three lines of skeleton content with varying widths
- Customizable styling through className prop
- Consistent spacing and appearance
- Built on top of the Skeleton UI component
        `,
      },
    },
  },
  // This component will have an automatically generated Autodocs entry: https://storybook.js.org/docs/writing-docs/autodocs
  tags: ['autodocs'],
  // More on argTypes: https://storybook.js.org/docs/api/argtypes
  argTypes: {
    className: {
      description: 'Additional CSS classes to apply to the skeleton card',
      control: { type: 'text' },
      type: { name: 'string', required: false },
    },
  },
  args: {
    className: '',
  },
} satisfies Meta<typeof SkeletonCard>;

export default meta;
type Story = StoryObj<typeof meta>;

// More on writing stories with args: https://storybook.js.org/docs/writing-stories/args

export const WithCustomWidth: Story = {
  args: {
    className: 'w-80',
  },
  parameters: {
    docs: {
      description: {
        story: `
### Custom Width

Shows the skeleton card with a custom width applied.

\`\`\`tsx
<SkeletonCard className="w-80" />
\`\`\`
        `,
      },
    },
  },
  tags: ['!dev'],
};
