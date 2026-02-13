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
import {
  Calendar as CalendarIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
} from 'lucide-react';
import {
  ChangeEvent,
  forwardRef,
  InputHTMLAttributes,
  useEffect,
  useMemo,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from './button';
import { TimePicker } from './time-picker';

type PickerType = 'date' | 'month' | 'year';

interface DatePickerProps extends Omit<
  InputHTMLAttributes<HTMLInputElement>,
  'value' | 'onChange'
> {
  value?: Date | number;
  onChange?: (date: Date | undefined) => void;
  showTimeSelect?: boolean;
  dateFormat?: string;
  timeFormat?: string;
  showTimeSelectOnly?: boolean;
  locale?: Locale;
  openChange?: (open: boolean) => void;
  picker?: PickerType;
  allowInput?: boolean;
  minYear?: number;
  maxYear?: number;
}

const DatePicker = forwardRef<HTMLInputElement, DatePickerProps>(
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
      picker = 'date',
      allowInput = false,
      minYear = 1900,
      maxYear = 2100,
      placeholder,
      ...props
    },
    ref,
  ) => {
    const { t } = useTranslation();
    const [open, setOpen] = useState(false);
    const [inputValue, setInputValue] = useState('');
    const [currentYear, setCurrentYear] = useState(() => {
      const year = value
        ? value instanceof Date
          ? value.getFullYear()
          : value
        : new Date().getFullYear();
      return year;
    });
    const [currentMonth, setCurrentMonth] = useState(() => {
      if (value instanceof Date) {
        return value.getMonth();
      }
      return new Date().getMonth();
    });

    const selectedDate = useMemo(() => {
      if (!value) return undefined;
      if (value instanceof Date) return value;
      return new Date(value, 0, 1);
    }, [value]);

    const displayFormat = useMemo(() => {
      if (picker === 'year') return 'YYYY';
      if (picker === 'month') return 'MM/YYYY';
      if (showTimeSelect) return `${dateFormat} ${timeFormat}`;
      if (showTimeSelectOnly) return timeFormat;
      return dateFormat;
    }, [picker, dateFormat, timeFormat, showTimeSelect, showTimeSelectOnly]);

    const formattedValue = useMemo(() => {
      if (selectedDate && !isNaN(selectedDate.getTime())) {
        return dayjs(selectedDate).format(displayFormat);
      }
      return inputValue || '';
    }, [selectedDate, displayFormat, inputValue]);

    useEffect(() => {
      if (selectedDate && !isNaN(selectedDate.getTime())) {
        setInputValue(dayjs(selectedDate).format(displayFormat));
      }
    }, [selectedDate, displayFormat]);

    const handleDateSelect = (date: Date | undefined) => {
      if (date && selectedDate) {
        const valueDate = dayjs(selectedDate);
        date.setHours(valueDate.hour());
        date.setMinutes(valueDate.minute());
        date.setSeconds(valueDate.second());
      }
      onChange?.(date);
    };

    const handleTimeSelect = (date: Date | undefined) => {
      const valueDate = dayjs(selectedDate);
      if (selectedDate) {
        date?.setFullYear(valueDate.year());
        date?.setMonth(valueDate.month());
        date?.setDate(valueDate.date());
      }
      if (date) {
        onChange?.(date);
      } else {
        valueDate?.hour(0);
        valueDate?.minute(0);
        valueDate?.second(0);
        onChange?.(valueDate.toDate());
      }
    };

    const handleInputChange = (e: ChangeEvent<HTMLInputElement>) => {
      const newValue = e.target.value;
      setInputValue(newValue);

      if (allowInput) {
        const parsed = dayjs(newValue, displayFormat, true);
        if (parsed.isValid()) {
          onChange?.(parsed.toDate());
        }
      }
    };

    const handleOpenChange = (open: boolean) => {
      setOpen(open);
      openChange?.(open);
    };

    const handleYearSelect = (year: number) => {
      if (picker === 'year') {
        onChange?.(new Date(year, 0, 1));
        setOpen(false);
      } else {
        setCurrentYear(year);
      }
    };

    const handleMonthSelect = (month: number) => {
      if (picker === 'month') {
        onChange?.(new Date(currentYear, month, 1));
        setOpen(false);
      } else {
        setCurrentMonth(month);
      }
    };

    const years = useMemo(() => {
      const result: number[] = [];
      const startYear = Math.floor(currentYear / 10) * 10 - 1;
      for (let i = startYear; i <= startYear + 12; i++) {
        result.push(i);
      }
      return result;
    }, [currentYear]);

    const months = useMemo(() => {
      return [
        { value: 0, label: t('common.january', 'Jan') },
        { value: 1, label: t('common.february', 'Feb') },
        { value: 2, label: t('common.march', 'Mar') },
        { value: 3, label: t('common.april', 'Apr') },
        { value: 4, label: t('common.may', 'May') },
        { value: 5, label: t('common.june', 'Jun') },
        { value: 6, label: t('common.july', 'Jul') },
        { value: 7, label: t('common.august', 'Aug') },
        { value: 8, label: t('common.september', 'Sep') },
        { value: 9, label: t('common.october', 'Oct') },
        { value: 10, label: t('common.november', 'Nov') },
        { value: 11, label: t('common.december', 'Dec') },
      ];
    }, [t]);

    const [view, setView] = useState<'date' | 'month' | 'year'>(
      picker === 'year' ? 'year' : picker === 'month' ? 'month' : 'date',
    );

    useEffect(() => {
      setView(
        picker === 'year' ? 'year' : picker === 'month' ? 'month' : 'date',
      );
    }, [picker]);

    const handlePrev = () => {
      if (view === 'year') {
        setCurrentYear((prev) => prev - 10);
      } else if (view === 'month') {
        setCurrentYear((prev) => prev - 1);
      } else {
        const newMonth = currentMonth === 0 ? 11 : currentMonth - 1;
        const newYear = currentMonth === 0 ? currentYear - 1 : currentYear;
        setCurrentMonth(newMonth);
        setCurrentYear(newYear);
      }
    };

    const handleNext = () => {
      if (view === 'year') {
        setCurrentYear((prev) => prev + 10);
      } else if (view === 'month') {
        setCurrentYear((prev) => prev + 1);
      } else {
        const newMonth = currentMonth === 11 ? 0 : currentMonth + 1;
        const newYear = currentMonth === 11 ? currentYear + 1 : currentYear;
        setCurrentMonth(newMonth);
        setCurrentYear(newYear);
      }
    };

    const renderYearPicker = () => (
      <div className="p-0">
        <div className="flex items-center justify-between mb-3">
          <Button
            variant="ghost"
            size="icon"
            className="size-7"
            onClick={handlePrev}
          >
            <ChevronLeftIcon className="size-4" />
          </Button>
          <span className="text-sm font-medium">
            {years[1]} - {years[10]}
          </span>
          <Button
            variant="ghost"
            size="icon"
            className="size-7"
            onClick={handleNext}
          >
            <ChevronRightIcon className="size-4" />
          </Button>
        </div>
        <div className="grid grid-cols-3 gap-2">
          {years.map((year) => {
            const isSelected = year === (selectedDate?.getFullYear() || 0);
            const isCurrentDecade = year >= years[1] && year <= years[10];
            const isDisabled = year < minYear || year > maxYear;

            return (
              <Button
                key={year}
                variant={isSelected ? 'default' : 'ghost'}
                size="lg"
                disabled={isDisabled}
                className={cn(
                  'text-sm ',
                  isSelected && 'bg-primary text-primary-foreground',
                  !isCurrentDecade && 'text-muted-foreground opacity-50',
                  !isSelected && isCurrentDecade && 'hover:bg-accent',
                )}
                onClick={() => handleYearSelect(year)}
              >
                {year}
              </Button>
            );
          })}
        </div>
      </div>
    );

    const renderMonthPicker = () => (
      <div className="p-3">
        <div className="flex items-center justify-between mb-3">
          <Button
            variant="ghost"
            size="icon"
            className="size-7"
            onClick={handlePrev}
          >
            <ChevronLeftIcon className="size-4" />
          </Button>
          <span
            className="text-sm font-medium cursor-pointer hover:text-primary"
            onClick={() => picker !== 'month' && setView('year')}
          >
            {currentYear}
          </span>
          <Button
            variant="ghost"
            size="icon"
            className="size-7"
            onClick={handleNext}
          >
            <ChevronRightIcon className="size-4" />
          </Button>
        </div>
        <div className="grid grid-cols-3 gap-2">
          {months.map((month) => {
            const isSelected =
              month.value === (selectedDate?.getMonth() ?? -1) &&
              currentYear === (selectedDate?.getFullYear() ?? 0);

            return (
              <Button
                key={month.value}
                variant={isSelected ? 'default' : 'ghost'}
                size="lg"
                className={cn(
                  'text-sm',
                  isSelected && 'bg-primary text-primary-foreground',
                )}
                onClick={() => handleMonthSelect(month.value)}
              >
                {month.label}
              </Button>
            );
          })}
        </div>
      </div>
    );

    const renderDatePicker = () => (
      <>
        <Calendar
          mode="single"
          selected={selectedDate}
          onSelect={handleDateSelect}
          month={new Date(currentYear, currentMonth)}
          onMonthChange={(date) => {
            setCurrentYear(date.getFullYear());
            setCurrentMonth(date.getMonth());
          }}
          onDayClick={() => {
            if (!showTimeSelect) {
              setOpen(false);
            }
          }}
          classNames={{
            month_caption:
              'relative mx-10 mb-1 flex h-9 items-center justify-center z-20 [&>span]:cursor-pointer',
          }}
          components={{
            Chevron: (props) => {
              if (props.orientation === 'left') {
                return (
                  <ChevronLeftIcon
                    size={16}
                    {...props}
                    aria-hidden="true"
                    onClick={(e) => {
                      e.stopPropagation();
                      handlePrev();
                    }}
                  />
                );
              }
              return (
                <ChevronRightIcon
                  size={16}
                  {...props}
                  aria-hidden="true"
                  onClick={(e) => {
                    e.stopPropagation();
                    handleNext();
                  }}
                />
              );
            },
          }}
        />
        {/* <div
          className="text-center py-1 text-sm cursor-pointer hover:text-primary"
          onClick={() => setView('month')}
        >
          {dayjs(new Date(currentYear, currentMonth)).format('MMMM YYYY')}
        </div> */}
      </>
    );

    const renderPicker = () => {
      if (view === 'year') return renderYearPicker();
      if (view === 'month') return renderMonthPicker();
      return renderDatePicker();
    };

    return (
      <div className="grid gap-2">
        <Popover open={open} onOpenChange={handleOpenChange}>
          <PopoverTrigger asChild>
            <div className="relative">
              <Input
                ref={ref}
                value={formattedValue}
                onChange={handleInputChange}
                readOnly={!allowInput}
                placeholder={placeholder}
                className={cn(
                  'bg-bg-card hover:text-text-primary border-border-button w-full justify-between px-3 font-normal outline-offset-0 outline-none focus-visible:outline-[3px]',
                  allowInput ? 'cursor-text' : 'cursor-pointer',
                  className,
                )}
                {...props}
              />
              <CalendarIcon
                className="absolute right-3 top-1/2 transform -translate-y-1/2 text-muted-foreground/80 group-hover:text-foreground shrink-0 transition-colors pointer-events-none"
                size={16}
              />
            </div>
          </PopoverTrigger>
          <PopoverContent className="w-auto p-2" align="start">
            {renderPicker()}
            {showTimeSelect && picker === 'date' && (
              <TimePicker
                value={selectedDate}
                onChange={(value: Date | undefined) => {
                  handleTimeSelect(value);
                }}
                showNow
              />
            )}
            <div className="w-full flex justify-end mt-2">
              <Button
                variant="ghost"
                type="button"
                className="text-sm mr-2"
                onClick={() => {
                  onChange?.(undefined);
                  setInputValue('');
                  handleOpenChange(false);
                }}
              >
                {t('common.clear', 'Clear')}
              </Button>
              <Button
                type="button"
                className="text-sm text-text-primary-inverse"
                onClick={() => {
                  handleOpenChange(false);
                }}
              >
                {t('common.confirm', 'OK')}
              </Button>
            </div>
          </PopoverContent>
        </Popover>
      </div>
    );
  },
);

DatePicker.displayName = 'DatePicker';

export { DatePicker };
