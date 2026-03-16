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
import { useTranslation } from 'react-i18next';
import { Button } from './button';
import { TimePicker } from './time-picker';
// import TimePicker from 'react-time-picker';
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
  locale?: Locale; // Support for internationalization
  openChange?: (open: boolean) => void;
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
      openChange,
      ...props
    },
    ref,
  ) => {
    const { t } = useTranslation();
    const [selectedDate, setSelectedDate] = React.useState<Date | undefined>(
      value,
    );
    const [open, setOpen] = React.useState(false);

    const handleDateSelect = (date: Date | undefined) => {
      if (selectedDate) {
        const valueDate = dayjs(selectedDate);
        date?.setHours(valueDate.hour());
        date?.setMinutes(valueDate.minute());
        date?.setSeconds(valueDate.second());
      }
      setSelectedDate(date);
      // onChange?.(date);
    };

    const handleTimeSelect = (date: Date | undefined) => {
      const valueDate = dayjs(selectedDate);
      if (selectedDate) {
        date?.setFullYear(valueDate.year());
        date?.setMonth(valueDate.month());
        date?.setDate(valueDate.date());
      }
      if (date) {
        // onChange?.(date);
        setSelectedDate(date);
      } else {
        valueDate?.hour(0);
        valueDate?.minute(0);
        valueDate?.second(0);
        // onChange?.(valueDate.toDate());
        setSelectedDate(valueDate.toDate());
      }
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
      return selectedDate && !isNaN(selectedDate.getTime())
        ? dayjs(selectedDate).format(displayFormat)
        : '';
    }, [selectedDate, displayFormat]);

    const handleOpenChange = (open: boolean) => {
      setOpen(open);
      openChange?.(open);
    };

    return (
      <div className="grid gap-2">
        <Popover
          open={open}
          onOpenChange={handleOpenChange}
          disableOutsideClick
        >
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
              selected={selectedDate}
              onSelect={handleDateSelect}
            />
            {showTimeSelect && (
              <TimePicker
                value={selectedDate}
                onChange={(value: Date | undefined) => {
                  handleTimeSelect(value);
                }}
                showNow
              />
              // <TimePicker onChange={onChange} value={value} />
            )}
            <div className="w-full flex justify-end mt-2">
              <Button
                variant="ghost"
                type="button"
                className="text-sm mr-2"
                onClick={() => {
                  onChange?.(value);
                  handleOpenChange(false);
                }}
              >
                {t('common.cancel')}
              </Button>
              <Button
                type="button"
                className="text-sm text-text-primary-inverse "
                onClick={() => {
                  onChange?.(selectedDate);
                  handleOpenChange(false);
                }}
              >
                {t('common.confirm')}
              </Button>
            </div>
          </PopoverContent>
        </Popover>
      </div>
    );
  },
);

DateInput.displayName = 'DateInput';

export { DateInput };
