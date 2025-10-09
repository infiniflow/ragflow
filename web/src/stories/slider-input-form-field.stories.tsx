import { Form } from '@/components/ui/form';
import type { Meta, StoryObj } from '@storybook/react-webpack5';
import { useForm } from 'react-hook-form';

import { SliderInputFormField } from '@/components/slider-input-form-field';
import { FormLayout } from '@/constants/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';

// More on how to set up stories at: https://storybook.js.org/docs/writing-stories#default-export
const meta = {
  title: 'Example/SliderInputFormField',
  component: SliderInputFormField,
  parameters: {
    layout: 'centered',
    docs: {
      description: {
        component: `
## Component Description

SliderInputFormField is a form field component that combines a slider and a numeric input field.
It provides a user-friendly way to select numeric values within a specified range.        `,
      },
    },
  },
  tags: ['autodocs'],
  argTypes: {
    name: { control: 'text' },
    label: { control: 'text' },
    min: { control: 'number' },
    max: { control: 'number' },
    step: { control: 'number' },
    defaultValue: { control: 'number' },
    layout: {
      control: 'select',
      options: [FormLayout.Vertical, FormLayout.Horizontal],
    },
  },
  args: {
    name: 'sliderValue',
    label: 'Slider Value',
    min: 0,
    max: 100,
    step: 1,
    defaultValue: 50,
  },
} satisfies Meta<typeof SliderInputFormField>;

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
    name: 'sliderValue',
    label: 'Slider Value',
    min: 0,
    max: 100,
    step: 1,
    defaultValue: 50,
  },
  parameters: {
    docs: {
      description: {
        story: `
### Basic Usage

\`\`\`tsx
import { SliderInputFormField } from '@/components/slider-input-form-field';

<SliderInputFormField
  name="sliderValue"
  label="Slider Value"
  min={0}
  max={100}
  step={1}
  defaultValue={50}
/>
\`\`\`
        `,
      },
    },
  },
};

export const HorizontalLayout: Story = {
  decorators: [withFormProvider],
  args: {
    name: 'horizontalSlider',
    label: 'Horizontal Slider',
    min: 0,
    max: 200,
    step: 5,
    defaultValue: 100,
    layout: FormLayout.Horizontal,
  },
  parameters: {
    docs: {
      description: {
        story: `
### Horizontal Layout

\`\`\`tsx
import { SliderInputFormField } from '@/components/slider-input-form-field';
import { FormLayout } from '@/constants/form';

<SliderInputFormField
  name="horizontalSlider"
  label="Horizontal Slider"
  min={0}
  max={200}
  step={5}
  defaultValue={100}
  layout={FormLayout.Horizontal}
/>
\`\`\`
        `,
      },
    },
  },
};

export const CustomRange: Story = {
  decorators: [withFormProvider],
  args: {
    name: 'customRange',
    label: 'Custom Range (0-1000)',
    min: 0,
    max: 1000,
    step: 10,
    defaultValue: 500,
  },
  parameters: {
    docs: {
      description: {
        story: `
### Custom Range

\`\`\`tsx
import { SliderInputFormField } from '@/components/slider-input-form-field';

<SliderInputFormField
  name="customRange"
  label="Custom Range (0-1000)"
  min={0}
  max={1000}
  step={10}
  defaultValue={500}
/>
\`\`\`
        `,
      },
    },
  },
};
