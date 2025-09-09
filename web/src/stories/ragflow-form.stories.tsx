import { zodResolver } from '@hookform/resolvers/zod';
import type { Meta, StoryObj } from '@storybook/react-webpack5';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';

// Define form schema
const FormSchema = z.object({
  username: z.string().min(2, {
    message: 'Username must be at least 2 characters.',
  }),
  email: z.string().email({
    message: 'Please enter a valid email address.',
  }),
  description: z.string().optional(),
});

// Create a wrapper component to demonstrate RAGFlowFormItem
function FormExample({
  horizontal = false,
  fieldName = 'username',
  label = 'Username',
  tooltip = 'Please enter your username',
  placeholder = 'Enter username',
}: {
  horizontal?: boolean;
  fieldName?: string;
  label?: string;
  tooltip?: string;
  placeholder?: string;
}) {
  const form = useForm({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      username: '',
      email: '',
      description: '',
    },
  });

  return (
    <div className="w-full p-4 border rounded-lg">
      <Form {...form}>
        <form className="space-y-4">
          <RAGFlowFormItem
            name={fieldName}
            label={label}
            tooltip={tooltip}
            horizontal={horizontal}
          >
            <Input placeholder={placeholder} />
          </RAGFlowFormItem>
        </form>
      </Form>
    </div>
  );
}

// More on how to set up stories at: https://storybook.js.org/docs/writing-stories#default-export
const meta = {
  title: 'Example/RAGFlowForm',
  component: FormExample,
  parameters: {
    // Optional parameter to center the component in the Canvas. More info: https://storybook.js.org/docs/configure/story-layout
    layout: 'centered',
    docs: {
      description: {
        component: `
## RAGFlowFormItem Component

RAGFlowFormItem is a wrapper component built on top of shadcn/ui Form components, providing unified form item styling and layout.

### Import Path
\`\`\`typescript
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Form } from '@/components/ui/form';
\`\`\`

### Basic Usage
\`\`\`tsx
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';

const FormSchema = z.object({
  username: z.string(),
});

function MyForm() {
  const form = useForm({
    resolver: zodResolver(FormSchema),
    defaultValues: { username: '' },
  });

  return (
    <Form {...form}>
      <form>
        <RAGFlowFormItem
          name="username"
          label="Username"
          tooltip="Please enter your username"
        >
          <Input placeholder="Enter username" />
        </RAGFlowFormItem>
      </form>
    </Form>
  );
}
\`\`\`

### Features
- Built-in FormField, FormItem, FormLabel, FormControl and FormMessage
- Supports both horizontal and vertical layouts
- Supports tooltip hints
- Fully compatible with react-hook-form
        `,
      },
    },
  },
  // This component will have an automatically generated Autodocs entry: https://storybook.js.org/docs/writing-docs/autodocs
  tags: ['autodocs'],
  // More on argTypes: https://storybook.js.org/docs/api/argtypes
  argTypes: {
    horizontal: {
      description: 'Whether to display the form item horizontally',
      control: { type: 'boolean' },
      type: { name: 'boolean', required: false },
      defaultValue: false,
    },
    fieldName: {
      description: 'The name of the form field',
      control: { type: 'text' },
      type: { name: 'string', required: true },
    },
    label: {
      description: 'The label of the form field',
      control: { type: 'text' },
      type: { name: 'string', required: false },
    },
    tooltip: {
      description: 'The tooltip text for the form field',
      control: { type: 'text' },
      type: { name: 'string', required: false },
    },
    placeholder: {
      description: 'The placeholder text for the input',
      control: { type: 'text' },
      type: { name: 'string', required: false },
    },
  },
  args: {
    horizontal: false,
    fieldName: 'username',
    label: 'Username',
    tooltip: 'Please enter your username',
    placeholder: 'Enter username',
  },
} satisfies Meta<typeof FormExample>;

export default meta;
type Story = StoryObj<typeof meta>;

// More on writing stories with args: https://storybook.js.org/docs/writing-stories/args
export const VerticalLayout: Story = {
  args: {
    horizontal: false,
    fieldName: 'username',
    label: 'Username',
    tooltip: 'Please enter your username',
    placeholder: 'Enter username',
  },
  parameters: {
    docs: {
      description: {
        story: `
### Vertical Layout Example

Default vertical layout with label above the input field.

\`\`\`tsx
<RAGFlowFormItem
  name="username"
  label="Username"
  tooltip="Please enter your username"
  horizontal={false}
>
  <Input placeholder="Enter username" />
</RAGFlowFormItem>
\`\`\`
        `,
      },
    },
  },
  // tags: ['!dev'],
};

export const HorizontalLayout: Story = {
  args: {
    horizontal: true,
    fieldName: 'email',
    label: 'Email Address',
    tooltip: 'Please enter a valid email address',
    placeholder: 'Enter email',
  },
  parameters: {
    docs: {
      description: {
        story: `
### Horizontal Layout Example

Horizontal layout with label and input field on the same row.

\`\`\`tsx
<RAGFlowFormItem
  name="email"
  label="Email Address"
  tooltip="Please enter a valid email address"
  horizontal={true}
>
  <Input type="email" placeholder="Enter email" />
</RAGFlowFormItem>
\`\`\`
        `,
      },
    },
  },
  // tags: ['!dev'],
};
