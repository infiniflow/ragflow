import { cn } from '@/lib/utils';
import { Clock } from 'lucide-react';
import * as React from 'react';
import { forwardRef } from 'react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';

interface DisabledTimes {
  disabledHours?: () => number[];
  disabledMinutes?: (hour: number) => number[];
  disabledSeconds?: (hour: number, minute: number) => number[];
}

interface TimePickerProps extends Omit<
  React.HTMLAttributes<HTMLDivElement>,
  'value' | 'onChange' | 'defaultValue'
> {
  value?: Date;
  onChange?: (date: Date | undefined) => void;
  format?: string; // Time display format
  disabled?: boolean;
  placeholder?: string;
  className?: string;
  hourStep?: number;
  minuteStep?: number;
  secondStep?: number;
  allowClear?: boolean; // Whether to show clear button
  autoFocus?: boolean; // Auto focus
  bordered?: boolean; // Whether to show border
  disabledTime?: () => DisabledTimes; // Disabled time options
  hideDisabledOptions?: boolean; // Hide disabled options
  inputReadOnly?: boolean; // Set input as readonly
  use12Hours?: boolean; // Use 12-hour format
  size?: 'large' | 'middle' | 'small'; // Input size
  status?: 'error' | 'warning'; // Validation status
  onOpenChange?: (open: boolean) => void; // Callback when panel opens/closes
  open?: boolean; // Whether panel is open
  popupClassName?: string; // Popup class name
  popupStyle?: React.CSSProperties; // Popup style object
  renderExtraFooter?: () => React.ReactNode; // Custom content at bottom of picker
  showNow?: boolean; // Whether panel shows "Now" button
  defaultValue?: Date; // Default time
  cellRender?: (
    current: number,
    info: {
      originNode: React.ReactNode;
      today: Date;
      range?: 'start' | 'end';
      subType: 'hour' | 'minute' | 'second' | 'meridiem';
    },
  ) => React.ReactNode; // Customize cell content
  suffixIcon?: React.ReactNode; // Custom suffix icon
  clearIcon?: React.ReactNode; // Custom clear icon
  addon?: () => React.ReactNode; // Extra popup content
  minuteOptions?: number[]; // Minute options
  secondOptions?: number[]; // Second options
  placement?: 'bottomLeft' | 'bottomRight' | 'topLeft' | 'topRight'; // Placement of picker popup
}

// Scroll picker component
interface ScrollPickerProps {
  options: string[];
  value: string;
  onChange: (value: string) => void;
  disabledOptions?: number[];
  className?: string;
}

const ScrollPicker = React.memo<ScrollPickerProps>(
  ({ options, value, onChange, disabledOptions = [], className }) => {
    const containerRef = React.useRef<HTMLDivElement>(null);
    const selectedItemRef = React.useRef<HTMLDivElement>(null);

    // Scroll to the selected item and make it top
    React.useEffect(() => {
      if (containerRef.current && selectedItemRef.current) {
        const container = containerRef.current;
        const selectedItem = selectedItemRef.current;
        const itemHeight = selectedItem.clientHeight;
        const itemIndex = options.indexOf(value);

        if (itemIndex !== -1) {
          // Calculate the scroll distance to make the selected item top
          const scrollTop = itemIndex * itemHeight;
          container.scrollTop = scrollTop;
        }
      }
    }, [value, options]);

    return (
      <div className={cn('relative h-48 overflow-hidden', className)}>
        <div
          ref={containerRef}
          // onWheel={handleScroll}
          className="h-full overflow-y-auto scrollbar-none hover:scrollbar-auto"
        >
          {options.map((option, index) => {
            const isDisabled = disabledOptions.includes(index);
            const isSelected = option === value;
            return (
              <div
                key={`${option}-${index}`}
                ref={isSelected ? selectedItemRef : null}
                onClick={() => {
                  if (!isDisabled) onChange(option);
                }}
                className={cn(
                  'h-8 flex items-center justify-center cursor-pointer text-sm',
                  'transition-colors duration-150',
                  {
                    'text-text-primary bg-bg-card': isSelected,
                    'text-text-disabled hover:bg-bg-card':
                      !isSelected && !isDisabled,
                    'text-text-disabled cursor-not-allowed opacity-50':
                      isDisabled,
                  },
                )}
              >
                {option}
              </div>
            );
          })}
          <div className="h-[calc(100%-32px)]"></div>
        </div>
        {/* Add a transparent top bar for visual distinction */}
        {/* <div className="absolute top-0 left-0 right-0 h-8 border-b border-gray-600 pointer-events-none"></div> */}
      </div>
    );
  },
);
ScrollPicker.displayName = 'ScrollPicker';

const TimePicker = forwardRef<HTMLDivElement, TimePickerProps>(
  (
    {
      value,
      onChange,
      format = 'HH:mm:ss',
      disabled = false,
      placeholder = 'Select time',
      className,
      hourStep = 1,
      minuteStep = 1,
      secondStep = 1,
      allowClear = false,
      autoFocus = false,
      bordered = true,
      disabledTime,
      hideDisabledOptions = false,
      inputReadOnly = true,
      use12Hours = false,
      size = 'middle',
      status,
      onOpenChange,
      open,
      popupClassName,
      popupStyle,
      renderExtraFooter,
      showNow = false,
      defaultValue,
      placement = 'bottomLeft',
      suffixIcon,
      clearIcon,
      addon,
      minuteOptions,
      secondOptions,
      ...props
    },
    ref,
  ) => {
    const [isOpen, setIsOpen] = React.useState(false);
    const [inputValue, setInputValue] = React.useState('');
    const internalOpen = open !== undefined ? open : isOpen;

    // Initialize default value
    React.useEffect(() => {
      if (!value && defaultValue) {
        onChange?.(defaultValue);
      }
    }, [value, defaultValue, onChange]);

    // Update input field value
    React.useEffect(() => {
      if (value) {
        const hours = value.getHours();
        const minutes = value.getMinutes();
        const seconds = value.getSeconds();

        // Format output according to format
        let formatted = '';

        if (use12Hours) {
          let displayHour = hours % 12;
          if (displayHour === 0) displayHour = 12;

          formatted = `${String(displayHour).padStart(2, '0')}`;

          if (format.toLowerCase().includes('mm')) {
            formatted += `:${String(minutes).padStart(2, '0')}`;
          }

          if (format.toLowerCase().includes('ss')) {
            formatted += `:${String(seconds).padStart(2, '0')}`;
          }

          if (format.toLowerCase().includes('a')) {
            formatted += ` ${hours >= 12 ? 'PM' : 'AM'}`;
          } else if (format.toLowerCase().includes('A')) {
            formatted += ` ${hours >= 12 ? 'pm' : 'am'}`;
          }
        } else {
          formatted = String(hours).padStart(2, '0');

          if (format.toLowerCase().includes('mm')) {
            formatted += `:${String(minutes).padStart(2, '0')}`;
          }

          if (format.toLowerCase().includes('ss')) {
            formatted += `:${String(seconds).padStart(2, '0')}`;
          }
        }

        setInputValue(formatted);
      } else {
        setInputValue('');
      }
    }, [value, format, use12Hours]);

    const handleOpenChange = (newOpen: boolean) => {
      setIsOpen(newOpen);
      onOpenChange?.(newOpen);
    };

    // Handle input field changes
    const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
      const newValue = e.target.value;
      setInputValue(newValue);

      // Try to parse the input time string
      if (newValue) {
        const parsedDate = parseTimeInput(newValue, format, use12Hours);
        if (parsedDate) {
          onChange?.(parsedDate);
        }
      } else {
        onChange?.(undefined);
      }
    };

    // Parse time input string
    const parseTimeInput = (
      input: string,
      fmt: string,
      use12H: boolean,
    ): Date | undefined => {
      // Remove spaces and normalize input
      const normalizedInput = input.trim();

      // Define time regular expression pattern
      let regex: RegExp;
      // let hasAmPm = false;
      if (use12H) {
        if (fmt.toLowerCase().includes('ss')) {
          regex = /^(\d{1,2}):(\d{1,2}):(\d{1,2})\s*(AM|PM|am|pm)?$/;
          // hasAmPm = true;
        } else {
          regex = /^(\d{1,2}):(\d{1,2})\s*(AM|PM|am|pm)?$/;
          // hasAmPm = true;
        }
      } else {
        if (fmt.toLowerCase().includes('ss')) {
          regex = /^(\d{1,2}):(\d{1,2}):(\d{1,2})$/;
        } else {
          regex = /^(\d{1,2}):(\d{1,2})$/;
        }
      }

      const match = normalizedInput.match(regex);
      if (!match) return undefined;

      const [hourStr, minuteStr, secondStr, ampmStr] = match;

      let hour = parseInt(hourStr, 10);
      const minute = parseInt(minuteStr, 10);
      const second = secondStr ? parseInt(secondStr, 10) : 0;

      // Validate time range
      if (minute >= 60 || second >= 60) return undefined;

      if (use12H && ampmStr) {
        const isPM = ampmStr.toLowerCase() === 'pm';
        if (hour < 1 || hour > 12) return undefined;

        if (isPM && hour !== 12) {
          hour += 12;
        } else if (!isPM && hour === 12) {
          hour = 0;
        }
      } else if (!use12H) {
        if (hour > 23) return undefined;
      }

      const newDate = value ? new Date(value) : new Date();
      newDate.setHours(hour, minute, second, 0);

      return newDate;
    };

    // Determine whether to show seconds based on format
    const showSeconds = format.toLowerCase().includes('ss');

    // Handle time changes
    const handleHourChange = (hourStr: string) => {
      let hour = parseInt(hourStr, 10);

      // Convert to 24-hour format if using 12-hour format
      if (use12Hours) {
        const currentHour = value?.getHours() || 0;
        const isAM = currentHour < 12;

        if (hour === 12) {
          hour = isAM ? 0 : 12;
        } else {
          hour = isAM
            ? parseInt(hour.toString(), 10)
            : parseInt(hour.toString(), 10) + 12;
        }
      }

      const newDate = value ? new Date(value) : new Date();
      newDate.setHours(hour);
      onChange?.(newDate);
    };

    const handleMinuteChange = (minuteStr: string) => {
      // if (!value) return;

      const minute = parseInt(minuteStr, 10);
      const newDate = value ? new Date(value) : new Date();
      newDate.setMinutes(minute);

      onChange?.(newDate);
    };

    const handleSecondChange = (secondStr: string) => {
      // if (!value) return;

      const second = parseInt(secondStr, 10);
      const newDate = value ? new Date(value) : new Date();
      newDate.setSeconds(second);

      onChange?.(newDate);
    };

    const handleAmPmChange = (ampm: 'AM' | 'PM') => {
      if (!value || !use12Hours) return;

      const newDate = new Date(value);
      const currentHour = newDate.getHours();

      if (ampm === 'AM' && currentHour >= 12) {
        newDate.setHours(currentHour - 12);
      } else if (ampm === 'PM' && currentHour < 12) {
        newDate.setHours(currentHour + 12);
      }

      onChange?.(newDate);
    };

    const getDisabledTimes = React.useCallback(() => {
      if (!disabledTime)
        return {
          disabledHours: [] as number[],
          disabledMinutes: () => [] as number[],
          disabledSeconds: () => [] as number[],
        };

      const disabled = disabledTime();
      return {
        disabledHours: disabled.disabledHours?.() || [],
        disabledMinutes: disabled.disabledMinutes
          ? (hour: number) => disabled.disabledMinutes!(hour)
          : () => [] as number[],
        disabledSeconds: disabled.disabledSeconds
          ? (hour: number, minute: number) =>
              disabled.disabledSeconds!(hour, minute)
          : () => [] as number[],
      };
    }, [disabledTime]);

    // Generate time options
    const generateTimeOptions = (
      step: number,
      max: number,
      disabledOptions: number[] = [],
      customOptions?: number[],
    ) => {
      let options: number[];

      if (customOptions && customOptions.length > 0) {
        options = customOptions;
      } else {
        options = [];
        for (let i = 0; i <= max; i += step) {
          options.push(i);
        }
      }

      // Filter out disabled options
      if (hideDisabledOptions) {
        options = options.filter((option) => !disabledOptions.includes(option));
      }

      return options.map((num) => String(num).padStart(2, '0'));
    };

    const disabledTimes = getDisabledTimes();

    const hours = use12Hours
      ? generateTimeOptions(
          hourStep,
          11,
          disabledTimes.disabledHours.filter((h) => h <= 11),
        )
      : generateTimeOptions(hourStep, 23, disabledTimes.disabledHours);

    const minutes = generateTimeOptions(
      minuteStep,
      59,
      value ? disabledTimes.disabledMinutes(value.getHours()) : [],
      minuteOptions,
    );

    const seconds = generateTimeOptions(
      secondStep,
      59,
      value
        ? disabledTimes.disabledSeconds(value.getHours(), value.getMinutes())
        : [],
      secondOptions,
    );

    // Get current time value
    const hourValue = React.useMemo(() => {
      if (!value) return '00';

      let hour = value.getHours();

      if (use12Hours) {
        hour = hour % 12;
        if (hour === 0) hour = 12;
      }

      return hour.toString().padStart(2, '0');
    }, [value, use12Hours]);

    const minuteValue = React.useMemo(() => {
      if (!value) return '00';
      return value.getMinutes().toString().padStart(2, '0');
    }, [value]);

    const secondValue = React.useMemo(() => {
      if (!value) return '00';
      return value.getSeconds().toString().padStart(2, '0');
    }, [value]);

    const ampmValue = React.useMemo(() => {
      if (!value || !use12Hours) return 'AM';
      return value.getHours() >= 12 ? 'PM' : 'AM';
    }, [value, use12Hours]);

    // Handle clear operation
    const handleClear = () => {
      onChange?.(undefined);
      setInputValue('');
      handleOpenChange(false);
    };

    // Handle Now button
    const handleSetNow = () => {
      onChange?.(new Date());
      handleOpenChange(false);
    };

    return (
      <div ref={ref} className={cn('w-full', className)} {...props}>
        <Popover
          open={internalOpen}
          onOpenChange={handleOpenChange}
          modal={false} // Use non-modal dialog box, consistent with Ant Design behavior
        >
          <PopoverTrigger asChild>
            <div className="relative">
              <Input
                type="text"
                value={inputValue}
                onChange={handleInputChange}
                placeholder={placeholder}
                disabled={disabled}
                readOnly={inputReadOnly}
                className={cn(
                  'pl-3 pr-8 py-2 font-normal',
                  size === 'large' && 'h-10 text-base',
                  size === 'small' && 'h-8 text-sm',
                  status === 'error' && 'border-red-500',
                  status === 'warning' && '!border-yellow-500',
                  !bordered && 'border-transparent',
                  'cursor-pointer',
                )}
                autoFocus={autoFocus}
              />
              <div className="absolute right-3 top-1/2 transform -translate-y-1/2 flex items-center">
                {allowClear && value && inputValue && (
                  <button
                    type="button"
                    onClick={(e) => {
                      e.stopPropagation();
                      handleClear();
                    }}
                    className="mr-2 text-muted-foreground hover:text-foreground"
                  >
                    {clearIcon || '✕'}
                  </button>
                )}

                <div className="cursor-pointer">
                  {suffixIcon || (
                    <Clock size={16} className="text-muted-foreground" />
                  )}
                </div>
              </div>
            </div>
          </PopoverTrigger>
          <PopoverContent
            className={cn('w-auto p-3', popupClassName)}
            align={
              (placement.replace(/top|bottom/, '').toLowerCase() as
                | 'start'
                | 'center'
                | 'end') || 'start'
            }
            side={placement.startsWith('top') ? 'top' : 'bottom'}
            style={popupStyle}
            avoidCollisions={true}
          >
            <div className="flex items-center space-x-0">
              {/* <Clock className="text-muted-foreground" size={16} /> */}
              <div className="flex space-x-2">
                {/* Hour selection */}
                <div className="flex flex-col">
                  <ScrollPicker
                    options={hours}
                    value={hourValue}
                    onChange={handleHourChange}
                    className="w-14"
                  />
                </div>

                {format.toLowerCase().includes('mm') && (
                  <>
                    {/* Minute selection */}
                    <div className="flex flex-col">
                      <ScrollPicker
                        options={minutes}
                        value={minuteValue}
                        onChange={handleMinuteChange}
                        className="w-14"
                      />
                    </div>
                  </>
                )}

                {showSeconds && (
                  <>
                    {/* Second selection */}
                    <div className="flex flex-col">
                      <ScrollPicker
                        options={seconds}
                        value={secondValue}
                        onChange={handleSecondChange}
                        className="w-14"
                      />
                    </div>
                  </>
                )}

                {use12Hours && (
                  <div className="flex flex-col ml-1">
                    <ScrollPicker
                      options={['AM', 'PM']}
                      value={ampmValue}
                      onChange={(val) => handleAmPmChange(val as 'AM' | 'PM')}
                      className="w-12"
                    />
                  </div>
                )}
              </div>
            </div>

            {/* Extra footer content */}
            {renderExtraFooter && (
              <div className="mt-2 pt-2 border-t border-border-button">
                {renderExtraFooter()}
              </div>
            )}

            {/* Now button */}
            {showNow && (
              <div className="mt-2 pt-2 border-t border-border-button">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleSetNow}
                  className="w-full text-xs"
                >
                  Now
                </Button>
              </div>
            )}

            {/* Addon content */}
            {addon && (
              <div className="mt-2 pt-2 border-t border-gray-200">
                {addon()}
              </div>
            )}
          </PopoverContent>
        </Popover>
      </div>
    );
  },
);

TimePicker.displayName = 'TimePicker';

export { TimePicker, type TimePickerProps };
