import type { Meta, StoryObj } from '@storybook/react-webpack5';

import { fn } from 'storybook/test';

import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';

// More on how to set up stories at: https://storybook.js.org/docs/writing-stories#default-export
const meta = {
  title: 'Example/ConfirmDeleteDialog',
  component: ConfirmDeleteDialog,
  parameters: {
    // Optional parameter to center the component in the Canvas. More info: https://storybook.js.org/docs/configure/story-layout
    layout: 'centered',
    docs: {
      description: {
        component: `
## Component Description

ConfirmDeleteDialog is a dialog component for confirming delete operations with customizable title and callback functions.        `,
      },
    },
  },
  // This component will have an automatically generated Autodocs entry: https://storybook.js.org/docs/writing-docs/autodocs
  // More on argTypes: https://storybook.js.org/docs/api/argtypes
  argTypes: {
    title: { control: 'text' },
    hidden: { control: 'boolean' },
    onOk: { action: 'onOk' },
    onCancel: { action: 'onCancel' },
  },
  // Use `fn` to spy on the onClick arg, which will appear in the actions panel once invoked: https://storybook.js.org/docs/essentials/actions#action-args
  args: { onOk: fn(), onCancel: fn() },
  tags: ['autodocs'],
} satisfies Meta<typeof ConfirmDeleteDialog>;

export default meta;
type Story = StoryObj<typeof meta>;

// More on writing stories with args: https://storybook.js.org/docs/writing-stories/args
export const Default: Story = {
  args: {
    title: 'Confirm Delete',
    children: <Button variant="destructive">Delete Item</Button>,
  },
  parameters: {
    docs: {
      description: {
        story: `
### Usage Examples

\`\`\`tsx
import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';

<ConfirmDeleteDialog
  title="Confirm Delete"
  onOk={() => console.log('Confirmed')}
  onCancel={() => console.log('Cancelled')}
>
  <Button variant="destructive">Delete Item</Button>
</ConfirmDeleteDialog>
\`\`\`
        `,
      },
    },
  },
};

export const WithCustomTitle: Story = {
  args: {
    title: 'Are you sure you want to delete this file?',
    children: <Button variant="destructive">Delete File</Button>,
  },
};
