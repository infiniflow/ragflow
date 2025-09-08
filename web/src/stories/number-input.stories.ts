import type { Meta, StoryObj } from '@storybook/react-webpack5';

import { fn } from 'storybook/test';

import NumberInput from '@/components/originui/number-input';

// More on how to set up stories at: https://storybook.js.org/docs/writing-stories#default-export
const meta = {
  title: 'Example/NumberInput',
  component: NumberInput,
  parameters: {
    // Optional parameter to center the component in the Canvas. More info: https://storybook.js.org/docs/configure/story-layout
    layout: 'centered',
    docs: {
      description: {
        component: `
## NumberInput Component

NumberInput is a numeric input component with increment/decrement buttons. It provides a user-friendly interface for entering numeric values with built-in validation and keyboard controls.

### Import Path
\`\`\`typescript
import NumberInput from '@/components/originui/number-input';
\`\`\`

### Basic Usage
\`\`\`tsx
import { useState } from 'react';
import NumberInput from '@/components/originui/number-input';

function MyComponent() {
  const [value, setValue] = useState(0);

  return (
    <NumberInput
      value={value}
      onChange={(newValue) => setValue(newValue)}
    />
  );
}
\`\`\`

### Features
- Increment/decrement buttons for easy value adjustment
- Keyboard input validation (only allows numeric input)
- Customizable height and styling
- Non-negative number validation
- Responsive design with Tailwind CSS
        `,
      },
    },
  },
  // This component will have an automatically generated Autodocs entry: https://storybook.js.org/docs/writing-docs/autodocs
  tags: ['autodocs'],
  // More on argTypes: https://storybook.js.org/docs/api/argtypes
  argTypes: {
    value: {
      description: 'The current numeric value',
      control: { type: 'number' },
    },
    onChange: {
      description: 'Callback function called when value changes',
      control: false,
    },
    height: {
      description: 'Custom height for the input component',
      control: { type: 'text' },
    },
    className: {
      description: 'Additional CSS classes for styling',
      control: { type: 'text' },
    },
  },
  // Use `fn` to spy on the onChange arg, which will appear in the actions panel once invoked: https://storybook.js.org/docs/essentials/actions#action-args
  args: { onChange: fn() },
} satisfies Meta<typeof NumberInput>;

export default meta;
type Story = StoryObj<typeof meta>;

// More on writing stories with args: https://storybook.js.org/docs/writing-stories/args
export const Default: Story = {
  args: {
    value: 0,
  },
  parameters: {
    docs: {
      description: {
        story: `
### Default Number Input

Shows the basic number input with default styling and zero value.

\`\`\`tsx
<NumberInput
  value={0}
  onChange={(value) => console.log('Value changed:', value)}
/>
\`\`\`
        `,
      },
    },
  },
  tags: ['!dev'],
};

export const WithInitialValue: Story = {
  args: {
    value: 10,
  },
  parameters: {
    docs: {
      description: {
        story: `
### With Initial Value

Shows the number input with a predefined initial value.

\`\`\`tsx
<NumberInput
  value={10}
  onChange={(value) => console.log('Value changed:', value)}
/>
\`\`\`
        `,
      },
    },
  },
  tags: ['!dev'],
};

export const CustomHeight: Story = {
  args: {
    value: 5,
    height: '60px',
  },
  parameters: {
    docs: {
      description: {
        story: `
### Custom Height

Shows the number input with custom height styling.

\`\`\`tsx
<NumberInput
  value={5}
  height="60px"
  onChange={(value) => console.log('Value changed:', value)}
/>
\`\`\`
        `,
      },
    },
  },
  tags: ['!dev'],
};

export const WithCustomClass: Story = {
  args: {
    value: 3,
    className: 'border-blue-500 bg-blue-50',
  },
  parameters: {
    docs: {
      description: {
        story: `
### With Custom Styling

Shows the number input with custom CSS classes for styling.

\`\`\`tsx
<NumberInput
  value={3}
  className="border-blue-500 bg-blue-50"
  onChange={(value) => console.log('Value changed:', value)}
/>
\`\`\`
        `,
      },
    },
  },
  tags: ['!dev'],
};
