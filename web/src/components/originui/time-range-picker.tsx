import { Calendar, DateRange } from '@/components/originui/calendar';
import { Button } from '@/components/ui/button';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { cn } from '@/lib/utils';
import {
  endOfDay,
  endOfMonth,
  endOfYear,
  format,
  startOfDay,
  startOfMonth,
  startOfYear,
  subDays,
  subMonths,
  subYears,
} from 'date-fns';
import { CalendarIcon } from 'lucide-react';
import { useEffect, useId, useState } from 'react';

const CalendarComp = ({
  selectDateRange,
  onSelect,
  ...props
}: ITimeRangePickerProps) => {
  const today = new Date();
  const yesterday = {
    from: startOfDay(subDays(today, 1)),
    to: endOfDay(subDays(today, 1)),
  };
  const last7Days = {
    from: startOfDay(subDays(today, 6)),
    to: endOfDay(today),
  };
  const last30Days = {
    from: startOfDay(subDays(today, 29)),
    to: endOfDay(today),
  };
  const monthToDate = {
    from: startOfMonth(today),
    to: endOfDay(today),
  };
  const lastMonth = {
    from: startOfMonth(subMonths(today, 1)),
    to: endOfMonth(subMonths(today, 1)),
  };
  const yearToDate = {
    from: startOfYear(today),
    to: endOfDay(today),
  };
  const lastYear = {
    from: startOfYear(subYears(today, 1)),
    to: endOfYear(subYears(today, 1)),
  };
  const dateRangeList = [
    { key: 'yestday', value: yesterday },
    { key: 'last7Days', value: last7Days },
    { key: 'last30Days', value: last30Days },
    { key: 'monthToDate', value: monthToDate },
    { key: 'lastMonth', value: lastMonth },
    { key: 'yearToDate', value: yearToDate },
    { key: 'lastYear', value: lastYear },
  ];
  const [month, setMonth] = useState(today);
  const [date, setDate] = useState<DateRange>(selectDateRange || last7Days);

  return (
    <div>
      <div className="rounded-md border">
        <div className="flex max-sm:flex-col">
          <div className="relative py-4 max-sm:order-1 max-sm:border-t sm:w-32">
            <div className="h-full sm:border-e">
              <div className="flex flex-col px-2 gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  className="w-full justify-start"
                  onClick={() => {
                    const newDateRange = {
                      from: startOfDay(today),
                      to: endOfDay(today),
                    };
                    setDate(newDateRange);
                    setMonth(today);
                    onSelect?.(newDateRange);
                  }}
                >
                  Today
                </Button>
                {dateRangeList.map((dateRange) => (
                  <Button
                    key={dateRange.key}
                    variant="ghost"
                    size="sm"
                    className="w-full justify-start"
                    onClick={() => {
                      setDate(dateRange.value);
                      setMonth(dateRange.value.to);
                      onSelect?.(dateRange.value);
                    }}
                  >
                    {dateRange.key}
                  </Button>
                ))}
              </div>
            </div>
          </div>
          <Calendar
            mode="range"
            selected={date}
            onSelect={(newDate) => {
              if (newDate) {
                const dateRange = newDate as DateRange;
                const newDateRange = {
                  from: startOfDay(dateRange.from),
                  to: dateRange.to ? endOfDay(dateRange.to) : undefined,
                };
                setDate(newDateRange);
                onSelect?.(newDateRange);
              }
            }}
            month={month}
            onMonthChange={setMonth}
            className="p-2"
            {...props}
            // disabled={[
            //   { after: today }, // Dates before today
            // ]}
          />
        </div>
      </div>
    </div>
  );
};

export type ITimeRangePickerProps = {
  onSelect: (e: DateRange) => void;
  selectDateRange?: DateRange;
  className?: string;
};
const TimeRangePicker = ({
  onSelect,
  selectDateRange,
  ...props
}: ITimeRangePickerProps) => {
  const id = useId();
  const today = new Date();

  // Initialize without timezone conversion
  const [date, setDate] = useState<DateRange | undefined>(
    selectDateRange
      ? {
          from: startOfDay(selectDateRange.from),
          to: selectDateRange.to ? endOfDay(selectDateRange.to) : undefined,
        }
      : {
          from: startOfDay(today),
          to: endOfDay(today),
        },
  );

  useEffect(() => {
    if (!selectDateRange || !selectDateRange.from) return;

    try {
      const fromDate = new Date(selectDateRange.from);
      const toDate = selectDateRange.to
        ? new Date(selectDateRange.to)
        : undefined;

      if (isNaN(fromDate.getTime())) return;

      if (toDate && isNaN(toDate.getTime())) return;

      setDate({
        from: startOfDay(fromDate),
        to: toDate ? endOfDay(toDate) : undefined,
      });
    } catch (error) {
      console.error('Error updating date range from props:', error);
    }
  }, [selectDateRange]);
  const onChange = (e: DateRange | undefined) => {
    if (!e) return;
    setDate(e);
    onSelect?.(e);
  };
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          id={id}
          variant="outline"
          className="group bg-muted-foreground/10 hover:bg-muted-foreground/10 border-input w-full justify-between px-3 font-normal outline-offset-0 outline-none focus-visible:outline-[3px]"
        >
          <span className={cn('truncate', !date && 'text-muted-foreground')}>
            {date?.from ? (
              date.to ? (
                <>
                  {format(date.from, 'LLL dd, y')} -{' '}
                  {format(date.to, 'LLL dd, y')}
                </>
              ) : (
                format(date.from, 'LLL dd, y')
              )
            ) : (
              'Pick a date range'
            )}
          </span>
          <CalendarIcon
            size={16}
            className="text-muted-foreground/80 group-hover:text-foreground shrink-0 transition-colors"
            aria-hidden="true"
          />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-auto p-2" align="start">
        <CalendarComp selectDateRange={date} onSelect={onChange} {...props} />
      </PopoverContent>
    </Popover>
  );
};
export default TimeRangePicker;
