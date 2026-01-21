import { Calendar } from '@/components/originui/calendar';
import { Input } from '@/components/ui/input';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { cn } from '@/lib/utils';
import { Locale } from 'date-fns';
import dayjs from 'dayjs';
import { Calendar as CalendarIcon } from 'lucide-react';
import * as React from 'react';

interface DateInputProps extends Omit<
  React.InputHTMLAttributes<HTMLInputElement>,
  'value' | 'onChange'
> {
  value?: Date;
  onChange?: (date: Date | undefined) => void;
  showTimeSelect?: boolean;
  dateFormat?: string;
  timeFormat?: string;
  showTimeSelectOnly?: boolean;
  showTimeInput?: boolean;
  timeInputLabel?: string;
  locale?: Locale; // Support for internationalization
}

const DateInput = React.forwardRef<HTMLInputElement, DateInputProps>(
  (
    {
      className,
      value,
      onChange,
      dateFormat = 'DD/MM/YYYY',
      timeFormat = 'HH:mm:ss',
      showTimeSelect = false,
      showTimeSelectOnly = false,
      showTimeInput = false,
      timeInputLabel = '',
      ...props
    },
    ref,
  ) => {
    const [open, setOpen] = React.useState(false);

    const handleDateSelect = (date: Date | undefined) => {
      onChange?.(date);
      setOpen(false);
    };

    // Determine display format based on the type of date picker
    let displayFormat = dateFormat;
    if (showTimeSelect) {
      displayFormat = `${dateFormat} ${timeFormat}`;
    } else if (showTimeSelectOnly) {
      displayFormat = timeFormat;
    }

    // Format the date according to the specified format
    const formattedValue = React.useMemo(() => {
      return value && !isNaN(value.getTime())
        ? dayjs(value).format(displayFormat)
        : '';
    }, [value, displayFormat]);

    return (
      <div className="grid gap-2">
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger asChild>
            <div className="relative">
              <Input
                ref={ref}
                value={formattedValue}
                readOnly
                className={cn(
                  'bg-bg-card hover:text-text-primary border-border-button w-full justify-between px-3 font-normal outline-offset-0 outline-none focus-visible:outline-[3px] cursor-pointer',
                  className,
                )}
                {...props}
              />
              <CalendarIcon
                className="absolute right-3 top-1/2 transform -translate-y-1/2 text-muted-foreground/80 group-hover:text-foreground shrink-0 transition-colors"
                size={16}
              />
            </div>
          </PopoverTrigger>
          <PopoverContent className="w-auto p-2" align="start">
            <Calendar
              mode="single"
              selected={value}
              onSelect={handleDateSelect}
              initialFocus
              {...(showTimeSelect && {
                showTimeInput,
                timeInputLabel,
              })}
            />
          </PopoverContent>
        </Popover>
      </div>
    );
  },
);

DateInput.displayName = 'DateInput';

export { DateInput };
