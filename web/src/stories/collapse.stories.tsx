import type { Meta, StoryObj } from '@storybook/react-webpack5';

import { fn } from 'storybook/test';

import { Collapse } from '@/components/collapse';
import { Button } from '@/components/ui/button';

// More on how to set up stories at: https://storybook.js.org/docs/writing-stories#default-export
const meta = {
  title: 'Example/Collapse',
  component: Collapse,
  parameters: {
    // Optional parameter to center the component in the Canvas. More info: https://storybook.js.org/docs/configure/story-layout
    layout: 'centered',
    docs: {
      description: {
        component: `
## Component Description

Collapse is a component that allows you to show or hide content with a smooth animation. It can be controlled or uncontrolled and supports custom titles and right-aligned content.

The component uses a trigger element (typically with an icon) to toggle the visibility of its content. It's built on top of Radix UI's Collapsible primitive.
        `,
      },
    },
  },
  // This component will have an automatically generated Autodocs entry: https://storybook.js.org/docs/writing-docs/autodocs
  // More on argTypes: https://storybook.js.org/docs/api/argtypes
  argTypes: {
    title: {
      control: 'text',
      description: 'The title text or element to display in the trigger',
    },
    open: {
      control: 'boolean',
      description: 'Controlled open state of the collapse',
    },
    defaultOpen: {
      control: 'boolean',
      description: 'Initial open state of the collapse',
    },
    disabled: {
      control: 'boolean',
      description: 'Whether the collapse is disabled',
    },
    rightContent: {
      control: 'text',
      description: 'Content to display on the right side of the trigger',
    },
    onOpenChange: {
      action: 'onOpenChange',
      description: 'Callback function when the open state changes',
    },
  },
  // Use `fn` to spy on the onClick arg, which will appear in the actions panel once invoked: https://storybook.js.org/docs/essentials/actions#action-args
  args: { onOpenChange: fn() },
} satisfies Meta<typeof Collapse>;

export default meta;
type Story = StoryObj<typeof meta>;

// More on writing stories with args: https://storybook.js.org/docs/writing-stories/args
export const Default: Story = {
  args: {
    title: 'Collapse Title',
    children: (
      <div className="p-4 border border-gray-200 rounded-md">
        <p>This is the collapsible content. It can be any React node.</p>
        <p>You can put any content here, including other components.</p>
      </div>
    ),
  },
  parameters: {
    docs: {
      description: {
        story: `
### Usage Examples

\`\`\`tsx
import { Collapse } from '@/components/collapse';

<Collapse title="Collapse Title">
  <div className="p-4 border border-gray-200 rounded-md">
    <p>This is the collapsible content.</p>
  </div>
</Collapse>
\`\`\`
        `,
      },
    },
  },
};

export const WithRightContent: Story = {
  args: {
    title: 'Collapse with Right Content',
    rightContent: <Button size="sm">Action</Button>,
    children: (
      <div className="p-4 border border-gray-200 rounded-md">
        <p>
          This collapse has additional content on the right side of the trigger.
        </p>
      </div>
    ),
  },
  parameters: {
    docs: {
      description: {
        story: `
### Usage Examples

\`\`\`tsx
import { Collapse } from '@/components/collapse';
import { Button } from '@/components/ui/button';

<Collapse 
  title="Collapse Title" 
  rightContent={<Button size="sm">Action</Button>}
>
  <div className="p-4 border border-gray-200 rounded-md">
    <p>Content with right-aligned action button.</p>
  </div>
</Collapse>
\`\`\`
        `,
      },
    },
  },
};

export const InitiallyClosed: Story = {
  args: {
    title: 'Initially Closed Collapse',
    defaultOpen: false,
    children: (
      <div className="p-4 border border-gray-200 rounded-md">
        <p>This collapse is initially closed.</p>
      </div>
    ),
  },
};

export const Disabled: Story = {
  args: {
    title: 'Disabled Collapse',
    disabled: true,
    children: (
      <div className="p-4 border border-gray-200 rounded-md">
        <p>This collapse is disabled and cannot be toggled.</p>
      </div>
    ),
  },
};
