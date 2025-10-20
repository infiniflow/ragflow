import { Calendar } from '@/components/originui/calendar';
import type { Meta, StoryObj } from '@storybook/react-webpack5';
import { useState } from 'react';

// More on how to set up stories at: https://storybook.js.org/docs/writing-stories#default-export
const meta = {
  title: 'Example/Calendar',
  component: Calendar,
  parameters: {
    // Optional parameter to center the component in the Canvas. More info: https://storybook.js.org/docs/configure/story-layout
    layout: 'centered',
    docs: {
      description: {
        component: `
## Calendar Component

Calendar is a date picker component based on react-day-picker that allows users to select dates or date ranges. It provides a clean and customizable interface for date selection with support for various customization options.

### Import Path
\`\`\`typescript
import { Calendar } from '@/components/originui/calendar';
\`\`\`

### Basic Usage
\`\`\`tsx
import { Calendar } from '@/components/originui/calendar';
import { useState } from 'react';

function MyComponent() {
  const [date, setDate] = useState<Date | undefined>(new Date());
  
  return (
    <Calendar
      mode="single"
      selected={date}
      onSelect={setDate}
      className="rounded-md border"
    />
  );
}
\`\`\`

### Features
- Single date selection
- Date range selection
- Customizable styling with className prop
- Navigation between months
- Today highlighting
- Disabled dates support
- Customizable components
- Built with Tailwind CSS
        `,
      },
    },
  },
  // This component will have an automatically generated Autodocs entry: https://storybook.js.org/docs/writing-docs/autodocs
  tags: ['autodocs'],
  // More on argTypes: https://storybook.js.org/docs/api/argtypes
  argTypes: {
    mode: {
      description: 'Selection mode - single date or range',
      control: { type: 'radio' },
      options: ['single', 'range'],
    },
    selected: {
      description: 'Selected date or date range',
      control: false,
    },
    onSelect: {
      description: 'Callback function when date is selected',
      control: false,
    },
    className: {
      description: 'Additional CSS classes for styling',
      control: { type: 'text' },
    },
    classNames: {
      description: 'Custom class names for internal elements',
      control: { type: 'object' },
    },
    showOutsideDays: {
      description: 'Whether to show outside days',
      control: { type: 'boolean' },
    },
    components: {
      description: 'Custom components for calendar elements',
      control: { type: 'object' },
    },
  },
} satisfies Meta<typeof Calendar>;

export default meta;
type Story = StoryObj<typeof meta>;

// More on writing stories with args: https://storybook.js.org/docs/writing-stories/args
export const Default: Story = {
  args: {
    showOutsideDays: true,
    className: 'rounded-md border',
  },
  render: () => {
    // eslint-disable-next-line react-hooks/rules-of-hooks
    const [date, setDate] = useState<Date | undefined>(new Date());

    return (
      <Calendar
        mode="single"
        selected={date}
        onSelect={setDate}
        showOutsideDays={true}
        className="rounded-md border"
      />
    );
  },
  parameters: {
    docs: {
      description: {
        story: `
### Default Calendar

Shows the basic calendar with single date selection mode.

\`\`\`tsx
const [date, setDate] = useState<Date | undefined>(new Date());

<Calendar
  mode="single"
  selected={date}
  onSelect={setDate}
  className="rounded-md border"
  showOutsideDays={true}
/>
\`\`\`
        `,
      },
    },
  },
};

export const RangeSelection: Story = {
  args: {
    showOutsideDays: true,
    className: 'rounded-md border',
  },
  render: () => {
    // eslint-disable-next-line react-hooks/rules-of-hooks
    const [range, setRange] = useState<{
      from: Date | undefined;
      to?: Date | undefined;
    }>({
      from: new Date(),
      to: undefined,
    });

    return (
      <Calendar
        mode="range"
        selected={range}
        onSelect={(range) =>
          setRange(range as { from: Date | undefined; to?: Date | undefined })
        }
        showOutsideDays={true}
        className="rounded-md border"
      />
    );
  },
  parameters: {
    docs: {
      description: {
        story: `
### Range Selection Calendar

Shows the calendar with date range selection mode.

\`\`\`tsx
const [range, setRange] = useState<{ from: Date | undefined; to?: Date | undefined }>({
  from: new Date(),
  to: undefined,
});

<Calendar
  mode="range"
  selected={range}
  onSelect={(date) => {
    if (!range.from) {
      setRange({ from: date });
    } else if (!range.to && date && date > range.from) {
      setRange({ from: range.from, to: date });
    } else {
      setRange({ from: date });
    }
  }}
  className="rounded-md border"
  showOutsideDays={true}
/>
\`\`\`
        `,
      },
    },
  },
};

export const WithoutOutsideDays: Story = {
  args: {
    showOutsideDays: false,
    className: 'rounded-md border',
  },
  render: () => {
    // eslint-disable-next-line react-hooks/rules-of-hooks
    const [date, setDate] = useState<Date | undefined>(new Date());

    return (
      <Calendar
        mode="single"
        selected={date}
        onSelect={setDate}
        showOutsideDays={false}
        className="rounded-md border"
      />
    );
  },
  parameters: {
    docs: {
      description: {
        story: `
### Calendar without Outside Days

Shows the calendar without displaying days from previous/next months.

\`\`\`tsx
const [date, setDate] = useState<Date | undefined>(new Date());

<Calendar
  mode="single"
  selected={date}
  onSelect={setDate}
  className="rounded-md border"
  showOutsideDays={false}
/>
\`\`\`
        `,
      },
    },
  },
};

export const CustomStyling: Story = {
  args: {
    showOutsideDays: true,
  },
  render: () => {
    // eslint-disable-next-line react-hooks/rules-of-hooks
    const [date, setDate] = useState<Date | undefined>(new Date());

    return (
      <Calendar
        mode="single"
        selected={date}
        onSelect={setDate}
        showOutsideDays={true}
        className="rounded-md border-2 border-primary bg-secondary"
        classNames={{
          caption_label: 'text-lg font-bold text-primary',
          day_button: 'size-10 rounded-full hover:bg-primary/20',
        }}
      />
    );
  },
  parameters: {
    docs: {
      description: {
        story: `
### Custom Styled Calendar

Shows the calendar with custom styling using className and classNames props.

\`\`\`tsx
const [date, setDate] = useState<Date | undefined>(new Date());

<Calendar
  mode="single"
  selected={date}
  onSelect={setDate}
  className="rounded-md border-2 border-primary bg-secondary"
  classNames={{
    caption_label: 'text-lg font-bold text-primary',
    day_button: 'size-10 rounded-full hover:bg-primary/20',
  }}
  showOutsideDays={true}
/>
\`\`\`
        `,
      },
    },
  },
};
