import {
  endOfMonth,
  endOfYear,
  format,
  startOfMonth,
  startOfYear,
  subDays,
  subMonths,
  subYears,
} from 'date-fns';
import { useEffect, useId, useState } from 'react';

import { Calendar, DateRange } from '@/components/originui/calendar';
import { Button } from '@/components/ui/button';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { cn } from '@/lib/utils';
import { CalendarIcon } from 'lucide-react';

const CalendarComp = ({
  selectDateRange,
  onSelect,
  ...props
}: ITimeRangePickerProps) => {
  const today = new Date();
  const yesterday = {
    from: subDays(today, 1),
    to: subDays(today, 1),
  };
  const last7Days = {
    from: subDays(today, 6),
    to: today,
  };
  const last30Days = {
    from: subDays(today, 29),
    to: today,
  };
  const monthToDate = {
    from: startOfMonth(today),
    to: today,
  };
  const lastMonth = {
    from: startOfMonth(subMonths(today, 1)),
    to: endOfMonth(subMonths(today, 1)),
  };
  const yearToDate = {
    from: startOfYear(today),
    to: today,
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
  useEffect(() => {
    onSelect?.(date);
  }, [date, onSelect]);
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
                    setDate({
                      from: today,
                      to: today,
                    });
                    setMonth(today);
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
                setDate(newDate as DateRange);
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
  selectDateRange: DateRange;
  className?: string;
};
const TimeRangePicker = ({
  onSelect,
  selectDateRange,
  ...props
}: ITimeRangePickerProps) => {
  const id = useId();
  const today = new Date();
  const [date, setDate] = useState<DateRange | undefined>(
    selectDateRange || { from: today, to: today },
  );
  useEffect(() => {
    setDate(selectDateRange);
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
