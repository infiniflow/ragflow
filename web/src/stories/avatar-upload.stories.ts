import type { Meta, StoryObj } from '@storybook/react-webpack5';

import { fn } from 'storybook/test';

import { AvatarUpload } from '@/components/avatar-upload';

// More on how to set up stories at: https://storybook.js.org/docs/writing-stories#default-export
const meta = {
  title: 'Example/AvatarUpload',
  component: AvatarUpload,
  parameters: {
    // Optional parameter to center the component in the Canvas. More info: https://storybook.js.org/docs/configure/story-layout
    layout: 'centered',
    docs: {
      description: {
        component: `
## AvatarUpload Component

AvatarUpload is a file upload component specifically designed for uploading and displaying avatar images. It supports image preview, removal, and provides a user-friendly interface for avatar management.

### Import Path
\`\`\`typescript
import { AvatarUpload } from '@/components/avatar-upload';
\`\`\`

### Basic Usage
\`\`\`tsx
import { useState } from 'react';
import { AvatarUpload } from '@/components/avatar-upload';

function MyComponent() {
  const [avatarValue, setAvatarValue] = useState('');

  return (
    <AvatarUpload
      value={avatarValue}
      onChange={(base64String) => setAvatarValue(base64String)}
    />
  );
}
\`\`\`

### Features
- Image preview with hover effects
- Remove button to clear selected image
- Base64 encoding for easy handling
- Accepts common image formats (jpg, jpeg, png, webp, bmp)
        `,
      },
    },
  },
  // This component will have an automatically generated Autodocs entry: https://storybook.js.org/docs/writing-docs/autodocs
  tags: ['autodocs'],
  // More on argTypes: https://storybook.js.org/docs/api/argtypes
  argTypes: {
    value: {
      description: 'The current avatar value as base64 string',
      control: { type: 'text' },
      type: { name: 'string', required: false },
    },
    onChange: {
      description: 'Callback function called when avatar changes',
      control: false,
      type: { name: 'function', required: false },
    },
  },
  // Use `fn` to spy on the onChange arg, which will appear in the actions panel once invoked: https://storybook.js.org/docs/essentials/actions#action-args
  args: { onChange: fn() },
} satisfies Meta<typeof AvatarUpload>;

export default meta;
type Story = StoryObj<typeof meta>;

// More on writing stories with args: https://storybook.js.org/docs/writing-stories/args
export const EmptyState: Story = {
  args: {
    value: '',
  },
  parameters: {
    docs: {
      description: {
        story: `
### Empty State

Shows the upload area when no avatar is selected.

\`\`\`tsx
<AvatarUpload
  value=""
  onChange={(base64String) => console.log('Avatar uploaded:', base64String)}
/>
\`\`\`
        `,
      },
    },
  },
  tags: ['!dev'],
};
