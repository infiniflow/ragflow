import { Form } from '@/components/ui/form';
import type { Meta, StoryObj } from '@storybook/react-webpack5';
import { useForm } from 'react-hook-form';

import { NodeCollapsible } from '@/components/collapse';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';

// More on how to set up stories at: https://storybook.js.org/docs/writing-stories#default-export
const meta = {
  title: 'Example/NodeCollapsible',
  component: NodeCollapsible,
  parameters: {
    layout: 'centered',
    docs: {
      description: {
        component: `
## Component Description

NodeCollapsible is a specialized component for displaying collapsible content within nodes. 
It automatically shows only the first 3 items and provides a toggle button to expand/collapse the rest.
The component is designed to work within the application's node-based UI, such as in agent or data flow canvases.

The toggle button is displayed as a small circle at the bottom center of the component when there are more than 3 items.
        `,
      },
    },
  },
  tags: ['autodocs'],
  argTypes: {
    items: {
      control: 'object',
      description: 'Array of items to display in the collapsible component',
    },
    children: {
      control: false,
      description: 'Function to render each item',
    },
    className: {
      control: 'text',
      description: 'Additional CSS classes to apply to the component',
    },
  },
} satisfies Meta<typeof NodeCollapsible>;

// Form wrapper decorator
const WithFormProvider = ({ children }: { children: React.ReactNode }) => {
  const form = useForm({
    defaultValues: {},
    resolver: zodResolver(z.object({})),
  });
  return <Form {...form}>{children}</Form>;
};

const withFormProvider = (Story: any) => (
  <WithFormProvider>
    <Story />
  </WithFormProvider>
);

export default meta;
type Story = StoryObj<typeof meta>;

// More on writing stories with args: https://storybook.js.org/docs/writing-stories/args
export const Default: Story = {
  decorators: [withFormProvider],
  args: {
    items: [
      'Document Analysis Parser',
      'Web Search Parser',
      'Database Query Parser',
      'Image Recognition Parser',
      'Audio Transcription Parser',
      'Video Processing Parser',
      'Code Analysis Parser',
      'Spreadsheet Parser',
    ],
    children: (item: string) => (
      <div className="px-4 py-2 border rounded-md bg-bg-component">{item}</div>
    ),
  },
  parameters: {
    docs: {
      description: {
        story: `
### Basic Usage

By default, the NodeCollapsible component shows the first 3 items and collapses the rest.
A toggle button appears at the bottom when there are more than 3 items.

\`\`\`tsx
import { NodeCollapsible } from '@/components/collapse';

<NodeCollapsible 
  items={[
    'Document Analysis Parser',
    'Web Search Parser', 
    'Database Query Parser',
    'Image Recognition Parser',
    'Audio Transcription Parser',
    'Video Processing Parser',
    'Code Analysis Parser',
    'Spreadsheet Parser'
  ]}
>
  {(item) => (
    <div className="px-4 py-2 border rounded-md bg-bg-component">
      {item}
    </div>
  )}
</NodeCollapsible>
\`\`\`
        `,
      },
    },
  },
};

export const WithFewItems: Story = {
  decorators: [withFormProvider],
  args: {
    items: ['Single Item'],
    children: (item: string) => (
      <div className="px-4 py-2 border rounded-md bg-bg-component">{item}</div>
    ),
  },
  parameters: {
    docs: {
      description: {
        story: `
When there are 3 or fewer items, no toggle button is shown.
        `,
      },
    },
  },
};

export const WithManyItems: Story = {
  decorators: [withFormProvider],
  args: {
    items: [
      'Item 1',
      'Item 2',
      'Item 3',
      'Item 4',
      'Item 5',
      'Item 6',
      'Item 7',
      'Item 8',
    ],
    children: (item: string) => (
      <div className="px-4 py-2 border rounded-md bg-bg-component">{item}</div>
    ),
  },
  parameters: {
    docs: {
      description: {
        story: `
When there are more than 3 items, a toggle button is shown at the bottom center.
Clicking it will expand to show all items.
        `,
      },
    },
  },
};
