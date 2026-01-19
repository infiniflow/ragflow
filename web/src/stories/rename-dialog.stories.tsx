import type { Meta, StoryObj } from '@storybook/react-webpack5';
import { useState } from 'react';
import { fn } from 'storybook/test';

import { RenameDialog } from '@/components/rename-dialog';
import { Button } from '@/components/ui/button';

// More on how to set up stories at: https://storybook.js.org/docs/writing-stories#default-export
const meta = {
  title: 'Example/RenameDialog',
  component: RenameDialog,
  parameters: {
    // Optional parameter to center the component in the Canvas. More info: https://storybook.js.org/docs/configure/story-layout
    layout: 'centered',
    docs: {
      description: {
        component: `
## Component Description

RenameDialog is a modal dialog component for renaming items. It provides a form with input validation and loading states, commonly used in chat applications for renaming conversations or creating new ones.

### Features
- Modal dialog with form input
- Loading state support
- Customizable title
- Initial name pre-filling
- Form validation and submission

### Import Path
\`\`\`tsx
import { RenameDialog } from '@/components/rename-dialog';
\`\`\`

### Basic Usage
\`\`\`tsx
import { RenameDialog } from '@/components/rename-dialog';
import { Button } from '@/components/ui/button';
import { useState } from 'react';

function MyComponent() {
  const [visible, setVisible] = useState(false);
  const [loading, setLoading] = useState(false);
  
  return (
    <div>
      <Button onClick={() => setVisible(true)}>
        Open Rename Dialog
      </Button>
      {visible && (
        <RenameDialog
          hideModal={() => setVisible(false)}
          onOk={async (name) => {
            setLoading(true);
            // Handle save logic
            console.log('New name:', name);
            setLoading(false);
            setVisible(false);
          }}
          initialName=""
          loading={loading}
        />
      )}
    </div>
  );
}
\`\`\`
        `,
      },
    },
  },
  // This component will have an automatically generated Autodocs entry: https://storybook.js.org/docs/writing-docs/autodocs
  tags: ['autodocs'],
  // More on argTypes: https://storybook.js.org/docs/api/argtypes
  argTypes: {
    initialName: {
      control: 'text',
      description: 'Initial name value for the input field',
    },
    title: {
      control: 'text',
      description: 'Custom title for the dialog',
    },
    loading: {
      control: 'boolean',
      description: 'Loading state of the save button',
    },
  },
  // Use `fn` to spy on the args, which will appear in the actions panel once invoked: https://storybook.js.org/docs/essentials/actions#action-args
  args: {
    hideModal: fn(),
    onOk: fn(),
  },
} satisfies Meta<typeof RenameDialog>;

export default meta;
type Story = StoryObj<typeof meta>;

// Story components to handle useState hooks
const DefaultStoryComponent = (args: any) => {
  const [visible, setVisible] = useState(false);

  return (
    <div>
      <Button onClick={() => setVisible(true)}>Open Rename Dialog</Button>
      {visible && (
        <RenameDialog
          {...args}
          hideModal={() => setVisible(false)}
          onOk={(name) => {
            args.onOk?.(name);
            setVisible(false);
          }}
        />
      )}
    </div>
  );
};

const WithInitialNameStoryComponent = (args: any) => {
  const [visible, setVisible] = useState(false);

  return (
    <div>
      <Button onClick={() => setVisible(true)}>
        Open Rename Dialog (with initial name)
      </Button>
      {visible && (
        <RenameDialog
          {...args}
          hideModal={() => setVisible(false)}
          onOk={(name) => {
            args.onOk?.(name);
            setVisible(false);
          }}
        />
      )}
    </div>
  );
};

const CreateNewChatStoryComponent = (args: any) => {
  const [visible, setVisible] = useState(false);

  return (
    <div>
      <Button onClick={() => setVisible(true)}>Create New Chat</Button>
      {visible && (
        <RenameDialog
          {...args}
          hideModal={() => setVisible(false)}
          onOk={(name) => {
            args.onOk?.(name);
            setVisible(false);
          }}
        />
      )}
    </div>
  );
};

const LoadingStateStoryComponent = (args: any) => {
  const [visible, setVisible] = useState(false);

  return (
    <div>
      <Button onClick={() => setVisible(true)}>
        Open Dialog (Loading State)
      </Button>
      {visible && (
        <RenameDialog
          {...args}
          hideModal={() => setVisible(false)}
          onOk={(name) => {
            args.onOk?.(name);
          }}
        />
      )}
    </div>
  );
};

// More on writing stories with args: https://storybook.js.org/docs/writing-stories/args
export const Default: Story = {
  render: (args) => <DefaultStoryComponent {...args} />,
  args: {
    initialName: '',
    loading: false,
  },
  parameters: {
    docs: {
      description: {
        story: `
### Default Rename Dialog

Basic rename dialog without initial name value. Click the button to open the dialog.
        `,
      },
    },
  },
};

export const WithInitialName: Story = {
  render: (args) => <WithInitialNameStoryComponent {...args} />,
  args: {
    initialName: 'My Chat Session',
    loading: false,
  },
  parameters: {
    docs: {
      description: {
        story: `
### Rename Dialog with Initial Name

Rename dialog pre-filled with an existing name for editing. Click the button to open the dialog.
        `,
      },
    },
  },
};

export const CreateNewChat: Story = {
  render: (args) => <CreateNewChatStoryComponent {...args} />,
  args: {
    initialName: '',
    title: 'Create Chat',
    loading: false,
  },
  parameters: {
    docs: {
      description: {
        story: `
### Create New Chat Dialog

Dialog for creating a new chat with custom title. Click the button to open the dialog.
        `,
      },
    },
  },
};

export const LoadingState: Story = {
  render: (args) => <LoadingStateStoryComponent {...args} />,
  args: {
    initialName: 'Saving changes...',
    loading: true,
  },
  parameters: {
    docs: {
      description: {
        story: `
### Loading State

Dialog showing loading state during save operation. The dialog remains open while loading.
        `,
      },
    },
  },
};
