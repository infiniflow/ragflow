import { Button } from '@/components/ui/button';
import { Modal } from '@/components/ui/modal/modal';
import type { Meta, StoryObj } from '@storybook/react-webpack5';
import { useState } from 'react';

// More on how to set up stories at: https://storybook.js.org/docs/writing-stories#default-export
const meta = {
  title: 'Example/Modal',
  component: Modal,
  parameters: {
    // Optional parameter to center the component in the Canvas. More info: https://storybook.js.org/docs/configure/story-layout
    layout: 'centered',
    docs: {
      description: {
        component: `
## Modal Component

The Modal component is a dialog overlay that can be used to present content in a modal window. It provides a flexible way to display information, forms, or any other content on top of the main page content.

### Import Path
\`\`\`typescript
import { Modal } from '@/components/ui/modal/modal';
\`\`\`

### Basic Usage
\`\`\`tsx
import { Modal } from '@/components/ui/modal/modal';
import { useState } from 'react';

function MyComponent() {
  const [open, setOpen] = useState(false);
  
  return (
    <>
      <button onClick={() => setOpen(true)}>Open Modal</button>
      <Modal 
        open={open} 
        onOpenChange={setOpen}
        title="Modal Title"
      >
        <p>Modal content goes here</p>
      </Modal>
    </>
  );
}
\`\`\`

### Features
- Multiple sizes: small, default, and large
- Customizable header with title and close button
- Customizable footer with default OK/Cancel buttons
- Support for controlled and uncontrolled usage
- Loading state for confirmation button
- Keyboard navigation support (ESC to close)
- Click outside to close functionality
- Full screen mode option
- Built with Radix UI primitives for accessibility
- Customizable styling with className props
        `,
      },
    },
  },
  // This component will have an automatically generated Autodocs entry: https://storybook.js.org/docs/writing-docs/autodocs
  tags: ['autodocs'],
  // More on argTypes: https://storybook.js.org/docs/api/argtypes
  argTypes: {
    open: {
      description: 'Whether the modal is open or not',
      control: { type: 'boolean' },
    },
    onOpenChange: {
      description:
        'Callback function that is called when the open state changes',
      control: false,
    },
    title: {
      description: 'Title of the modal',
      control: { type: 'text' },
    },
    titleClassName: {
      description: 'Additional CSS classes for the title container',
      control: { type: 'text' },
    },
    children: {
      description: 'Content to be displayed inside the modal',
      control: false,
    },
    footer: {
      description:
        'Custom footer content. If not provided, default buttons will be shown',
      control: { type: 'text' },
    },
    footerClassName: {
      description: 'Additional CSS classes for the footer container',
      control: { type: 'text' },
    },
    showfooter: {
      description: 'Whether to show the footer or not',
      control: { type: 'boolean' },
    },
    className: {
      description: 'Additional CSS classes for the modal container',
      control: { type: 'text' },
    },
    size: {
      description: 'Size of the modal',
      control: { type: 'select' },
      options: ['small', 'default', 'large'],
    },
    closable: {
      description: 'Whether to show the close button in the header',
      control: { type: 'boolean' },
    },
    closeIcon: {
      description: 'Custom close icon',
      control: false,
    },
    maskClosable: {
      description: 'Whether to close the modal when clicking on the mask',
      control: { type: 'boolean' },
    },
    destroyOnClose: {
      description: 'Whether to unmount the modal content when closed',
      control: { type: 'boolean' },
    },
    full: {
      description: 'Whether the modal should take the full screen',
      control: { type: 'boolean' },
    },
    confirmLoading: {
      description: 'Whether the confirm button should show a loading state',
      control: { type: 'boolean' },
    },
    cancelText: {
      description: 'Text for the cancel button',
      control: { type: 'text' },
    },
    okText: {
      description: 'Text for the OK button',
      control: { type: 'text' },
    },
    onOk: {
      description: 'Callback function for the OK button',
      control: false,
    },
    onCancel: {
      description: 'Callback function for the Cancel button',
      control: false,
    },
  },
} satisfies Meta<typeof Modal>;

export default meta;
type Story = StoryObj<typeof meta>;

// More on writing stories with args: https://storybook.js.org/docs/writing-stories/args

export const Default: Story = {
  args: {
    open: false,
    title: 'Default Modal',
    children: (
      <div>
        <h3 className="text-lg font-medium mb-2">Modal Content</h3>
        <p className="text-muted-foreground">
          This is the default modal with standard size and functionality.
        </p>
      </div>
    ),
  },
  render: (args) => {
    // eslint-disable-next-line react-hooks/rules-of-hooks
    const [open, setOpen] = useState(false);

    return (
      <div>
        <Button onClick={() => setOpen(true)}>Open Default Modal</Button>
        <Modal open={open} onOpenChange={setOpen} title={args.title}>
          {args.children}
        </Modal>
      </div>
    );
  },
  parameters: {
    docs: {
      description: {
        story: `
### Default Modal

Shows the basic modal with default size and standard header/footer.

\`\`\`tsx
const [open, setOpen] = useState(false);

<Button onClick={() => setOpen(true)}>Open Default Modal</Button>
<Modal 
  open={open} 
  onOpenChange={setOpen}
  title="Default Modal"
>
  <div>
    <h3 className="text-lg font-medium mb-2">Modal Content</h3>
    <p className="text-muted-foreground">
      This is the default modal with standard size and functionality.
    </p>
  </div>
</Modal>
\`\`\`
        `,
      },
    },
  },
};

export const Small: Story = {
  args: {
    open: false,
    title: 'Small Modal',
    size: 'small',
    children: (
      <div>
        <h3 className="text-lg font-medium mb-2">Small Modal</h3>
        <p className="text-muted-foreground">
          This is a small modal, suitable for simple confirmations or short
          messages.
        </p>
      </div>
    ),
  },
  render: (args) => {
    // eslint-disable-next-line react-hooks/rules-of-hooks
    const [open, setOpen] = useState(false);

    return (
      <div>
        <Button onClick={() => setOpen(true)}>Open Small Modal</Button>
        <Modal
          open={open}
          onOpenChange={setOpen}
          title={args.title}
          size={args.size}
        >
          {args.children}
        </Modal>
      </div>
    );
  },
  parameters: {
    docs: {
      description: {
        story: `
### Small Modal

Shows a small-sized modal, ideal for confirmations or brief messages.

\`\`\`tsx
const [open, setOpen] = useState(false);

<Button onClick={() => setOpen(true)}>Open Small Modal</Button>
<Modal 
  open={open} 
  onOpenChange={setOpen}
  title="Small Modal"
  size="small"
>
  <div>
    <h3 className="text-lg font-medium mb-2">Small Modal</h3>
    <p className="text-muted-foreground">
      This is a small modal, suitable for simple confirmations or short messages.
    </p>
  </div>
</Modal>
\`\`\`
        `,
      },
    },
  },
};

export const Large: Story = {
  args: {
    open: false,
    title: 'Large Modal',
    size: 'large',
    children: (
      <div>
        <h3 className="text-lg font-medium mb-2">Large Modal</h3>
        <p className="text-muted-foreground mb-4">
          This is a large modal with more content. It can accommodate forms,
          tables, or other complex content.
        </p>
        <div className="bg-muted p-4 rounded-md">
          <p>Additional content area</p>
          <p className="mt-2">You can put any content here</p>
        </div>
      </div>
    ),
  },
  render: (args) => {
    // eslint-disable-next-line react-hooks/rules-of-hooks
    const [open, setOpen] = useState(false);

    return (
      <div>
        <Button onClick={() => setOpen(true)}>Open Large Modal</Button>
        <Modal
          open={open}
          onOpenChange={setOpen}
          title={args.title}
          size={args.size}
        >
          {args.children}
        </Modal>
      </div>
    );
  },
  parameters: {
    docs: {
      description: {
        story: `
### Large Modal

Shows a large-sized modal, suitable for complex content like forms or data tables.

\`\`\`tsx
const [open, setOpen] = useState(false);

<Button onClick={() => setOpen(true)}>Open Large Modal</Button>
<Modal 
  open={open} 
  onOpenChange={setOpen}
  title="Large Modal"
  size="large"
>
  <div>
    <h3 className="text-lg font-medium mb-2">Large Modal</h3>
    <p className="text-muted-foreground mb-4">
      This is a large modal with more content. It can accommodate forms, tables, or other complex content.
    </p>
    <div className="bg-muted p-4 rounded-md">
      <p>Additional content area</p>
      <p className="mt-2">You can put any content here</p>
    </div>
  </div>
</Modal>
\`\`\`
        `,
      },
    },
  },
};

export const WithCustomFooter: Story = {
  args: {
    open: false,
    title: 'Custom Footer',
    children: (
      <div>
        <h3 className="text-lg font-medium mb-2">Modal with Custom Footer</h3>
        <p className="text-muted-foreground">
          This modal has a custom footer with multiple buttons.
        </p>
      </div>
    ),
  },
  render: (args) => {
    // eslint-disable-next-line react-hooks/rules-of-hooks
    const [open, setOpen] = useState(false);

    return (
      <div>
        <Button onClick={() => setOpen(true)}>
          Open Modal with Custom Footer
        </Button>
        <Modal
          open={open}
          onOpenChange={setOpen}
          title={args.title}
          footer={
            <div className="flex justify-between w-full">
              <Button variant="outline">Secondary Action</Button>
              <div className="space-x-2">
                <Button variant="outline" onClick={() => setOpen(false)}>
                  Cancel
                </Button>
                <Button>Primary Action</Button>
              </div>
            </div>
          }
        >
          {args.children}
        </Modal>
      </div>
    );
  },
  parameters: {
    docs: {
      description: {
        story: `
### Custom Footer

Shows a modal with a custom footer. You can provide your own footer content instead of using the default OK/Cancel buttons.

\`\`\`tsx
const [open, setOpen] = useState(false);

<Button onClick={() => setOpen(true)}>Open Modal with Custom Footer</Button>
<Modal 
  open={open} 
  onOpenChange={setOpen}
  title="Custom Footer"
  footer={
    <div className="flex justify-between w-full">
      <Button variant="outline">Secondary Action</Button>
      <div className="space-x-2">
        <Button variant="outline" onClick={() => setOpen(false)}>Cancel</Button>
        <Button>Primary Action</Button>
      </div>
    </div>
  }
>
  <div>
    <h3 className="text-lg font-medium mb-2">Modal with Custom Footer</h3>
    <p className="text-muted-foreground">
      This modal has a custom footer with multiple buttons.
    </p>
  </div>
</Modal>
\`\`\`
        `,
      },
    },
  },
};

export const WithoutFooter: Story = {
  args: {
    open: false,
    title: 'No Footer',
    children: (
      <div>
        <h3 className="text-lg font-medium mb-2">Modal without Footer</h3>
        <p className="text-muted-foreground">
          This modal has no footer. The content area extends to the bottom of
          the modal.
        </p>
      </div>
    ),
  },
  render: (args) => {
    // eslint-disable-next-line react-hooks/rules-of-hooks
    const [open, setOpen] = useState(false);

    return (
      <div>
        <Button onClick={() => setOpen(true)}>Open Modal without Footer</Button>
        <Modal
          open={open}
          onOpenChange={setOpen}
          title={args.title}
          showfooter={false}
        >
          {args.children}
        </Modal>
      </div>
    );
  },
  parameters: {
    docs: {
      description: {
        story: `
### Without Footer

Shows a modal without a footer. Useful when you want to include action buttons within the content area or don't need any footer actions.

\`\`\`tsx
const [open, setOpen] = useState(false);

<Button onClick={() => setOpen(true)}>Open Modal without Footer</Button>
<Modal 
  open={open} 
  onOpenChange={setOpen}
  title="No Footer"
  showfooter={false}
>
  <div>
    <h3 className="text-lg font-medium mb-2">Modal without Footer</h3>
    <p className="text-muted-foreground">
      This modal has no footer. The content area extends to the bottom of the modal.
    </p>
  </div>
</Modal>
\`\`\`
        `,
      },
    },
  },
};

export const FullScreen: Story = {
  args: {
    open: false,
    title: 'Full Screen Modal',
    children: (
      <div className="h-96 flex flex-col">
        <h3 className="text-lg font-medium mb-2">Full Screen Modal</h3>
        <p className="text-muted-foreground mb-4">
          This modal takes up the full screen. Useful for complex workflows or
          when you need maximum space.
        </p>
        <div className="flex-grow bg-muted rounded-md p-4">
          <p>Content area that can expand to fill available space</p>
        </div>
      </div>
    ),
  },
  render: (args) => {
    // eslint-disable-next-line react-hooks/rules-of-hooks
    const [open, setOpen] = useState(false);

    return (
      <div>
        <Button onClick={() => setOpen(true)}>Open Full Screen Modal</Button>
        <Modal
          open={open}
          onOpenChange={setOpen}
          title={args.title}
          full={true}
        >
          {args.children}
        </Modal>
      </div>
    );
  },
  parameters: {
    docs: {
      description: {
        story: `
### Full Screen Modal

Shows a full screen modal that takes up the entire viewport. Useful for complex workflows or when maximum space is needed.

\`\`\`tsx
const [open, setOpen] = useState(false);

<Button onClick={() => setOpen(true)}>Open Full Screen Modal</Button>
<Modal 
  open={open} 
  onOpenChange={setOpen}
  title="Full Screen Modal"
  full={true}
>
  <div className="h-96 flex flex-col">
    <h3 className="text-lg font-medium mb-2">Full Screen Modal</h3>
    <p className="text-muted-foreground mb-4">
      This modal takes up the full screen. Useful for complex workflows or when you need maximum space.
    </p>
    <div className="flex-grow bg-muted rounded-md p-4">
      <p>Content area that can expand to fill available space</p>
    </div>
  </div>
</Modal>
\`\`\`
        `,
      },
    },
  },
};

export const LoadingState: Story = {
  args: {
    open: false,
    title: 'Loading State',
    children: (
      <div>
        <h3 className="text-lg font-medium mb-2">Modal with Loading State</h3>
        <p className="text-muted-foreground">
          The OK button shows a loading spinner when confirmLoading is true.
        </p>
      </div>
    ),
  },
  render: (args) => {
    // eslint-disable-next-line react-hooks/rules-of-hooks
    const [open, setOpen] = useState(false);
    // eslint-disable-next-line react-hooks/rules-of-hooks
    const [loading, setLoading] = useState(false);

    const handleOk = () => {
      setLoading(true);
      setTimeout(() => {
        setLoading(false);
        setOpen(false);
      }, 2000);
    };

    return (
      <div>
        <Button onClick={() => setOpen(true)}>Open Loading State Modal</Button>
        <Modal
          open={open}
          onOpenChange={setOpen}
          title={args.title}
          confirmLoading={loading}
          onOk={handleOk}
        >
          {args.children}
        </Modal>
      </div>
    );
  },
  parameters: {
    docs: {
      description: {
        story: `
### Loading State

Shows a modal with the confirm button in a loading state. This is useful when performing async operations after clicking OK.

\`\`\`tsx
const [open, setOpen] = useState(false);
const [loading, setLoading] = useState(false);

const handleOk = () => {
  setLoading(true);
  setTimeout(() => {
    setLoading(false);
    setOpen(false);
  }, 2000);
};

<Button onClick={() => setOpen(true)}>Open Loading State Modal</Button>
<Modal 
  open={open} 
  onOpenChange={setOpen}
  title="Loading State"
  confirmLoading={loading}
  onOk={handleOk}
>
  <div>
    <h3 className="text-lg font-medium mb-2">Modal with Loading State</h3>
    <p className="text-muted-foreground">
      The OK button shows a loading spinner when confirmLoading is true.
    </p>
  </div>
</Modal>
\`\`\`
        `,
      },
    },
  },
};

// Interactive example showing how to use the modal in a real component

export const Interactive: Story = {
  args: {
    open: false,
    title: 'Interactive Modal',
    children: (
      <div>
        <h3 className="text-lg font-medium mb-2">Interactive Modal</h3>
        <p className="text-muted-foreground">
          Click OK to see the loading state, or click Cancel/Close to close the
          modal.
        </p>
      </div>
    ),
  },
  render: (args) => {
    // eslint-disable-next-line react-hooks/rules-of-hooks
    const [open, setOpen] = useState(false);

    return (
      <div>
        <Button onClick={() => setOpen(true)}>Open Interactive Modal</Button>
        <Modal
          open={open}
          onOpenChange={setOpen}
          title={args.title}
          onOk={() => {
            // Simulate API call
            setTimeout(() => {
              setOpen(false);
            }, 1000);
          }}
        >
          {args.children}
        </Modal>
      </div>
    );
  },
  parameters: {
    docs: {
      description: {
        story: `
### Interactive Example

This is a fully interactive example showing how to use the modal in a real component with state management.

\`\`\`tsx
import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Modal } from '@/components/ui/modal/modal';

function InteractiveModal() {
  const [open, setOpen] = useState(false);
  
  return (
    <div>
      <Button onClick={() => setOpen(true)}>Open Interactive Modal</Button>
      <Modal 
        open={open} 
        onOpenChange={setOpen}
        title="Interactive Modal"
        onOk={() => {
          // Simulate API call
          setTimeout(() => {
            setOpen(false);
          }, 1000);
        }}
      >
        <div>
          <h3 className="text-lg font-medium mb-2">Interactive Modal</h3>
          <p className="text-muted-foreground">
            Click OK to see the loading state, or click Cancel/Close to close the modal.
          </p>
        </div>
      </Modal>
    </div>
  );
}
\`\`\`
        `,
      },
    },
  },
};
