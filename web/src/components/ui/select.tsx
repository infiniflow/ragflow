'use client';

import * as SelectPrimitive from '@radix-ui/react-select';
import { Check, ChevronDown, ChevronUp, X } from 'lucide-react';
import * as React from 'react';

import { cn } from '@/lib/utils';
import { ControllerRenderProps } from 'react-hook-form';

import { FormControl } from '@/components/ui/form';
import { forwardRef, useCallback, useEffect } from 'react';

const Select = SelectPrimitive.Root;

const SelectGroup = SelectPrimitive.Group;

const SelectValue = SelectPrimitive.Value;

const SelectTrigger = React.forwardRef<
  React.ElementRef<typeof SelectPrimitive.Trigger>,
  React.ComponentPropsWithoutRef<typeof SelectPrimitive.Trigger> & {
    onReset?: () => void;
    allowClear?: boolean;
  }
>(({ className, children, value, onReset, allowClear, ...props }, ref) => (
  <SelectPrimitive.Trigger
    ref={ref}
    className={cn(
      'flex h-8 w-full items-center bg-bg-input justify-between rounded-md border-0.5 border-border-button',
      'px-3 py-2 text-sm outline-none transition-colors text-text-secondary placeholder:text-muted-foreground',
      'hover:text-text-primary hover:bg-border-button',
      'focus-visible:text-text-primary focus-visible:bg-border-button',
      'disabled:cursor-not-allowed disabled:opacity-50 [&>span]:line-clamp-1',
      className,
    )}
    {...props}
  >
    {children}
    <SelectPrimitive.Icon
      asChild
      onPointerDown={(event) => {
        event.stopPropagation();
      }}
    >
      {value && allowClear ? (
        <X className="h-4 w-4 opacity-50 cursor-pointer" onClick={onReset} />
      ) : (
        <ChevronDown className="h-4 w-4 opacity-50" />
      )}
    </SelectPrimitive.Icon>
  </SelectPrimitive.Trigger>
));
SelectTrigger.displayName = SelectPrimitive.Trigger.displayName;

const SelectScrollUpButton = React.forwardRef<
  React.ElementRef<typeof SelectPrimitive.ScrollUpButton>,
  React.ComponentPropsWithoutRef<typeof SelectPrimitive.ScrollUpButton>
>(({ className, ...props }, ref) => (
  <SelectPrimitive.ScrollUpButton
    ref={ref}
    className={cn(
      'flex cursor-default items-center justify-center py-1',
      className,
    )}
    {...props}
  >
    <ChevronUp className="h-4 w-4" />
  </SelectPrimitive.ScrollUpButton>
));
SelectScrollUpButton.displayName = SelectPrimitive.ScrollUpButton.displayName;

const SelectScrollDownButton = React.forwardRef<
  React.ElementRef<typeof SelectPrimitive.ScrollDownButton>,
  React.ComponentPropsWithoutRef<typeof SelectPrimitive.ScrollDownButton>
>(({ className, ...props }, ref) => (
  <SelectPrimitive.ScrollDownButton
    ref={ref}
    className={cn(
      'flex cursor-default items-center justify-center py-1',
      className,
    )}
    {...props}
  >
    <ChevronDown className="h-4 w-4" />
  </SelectPrimitive.ScrollDownButton>
));
SelectScrollDownButton.displayName =
  SelectPrimitive.ScrollDownButton.displayName;

const SelectContent = React.forwardRef<
  React.ElementRef<typeof SelectPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof SelectPrimitive.Content>
>(({ className, children, position = 'popper', ...props }, ref) => (
  <SelectPrimitive.Portal>
    <SelectPrimitive.Content
      ref={ref}
      className={cn(
        'relative z-50 max-h-96 min-w-[8rem] overflow-hidden rounded-md border-0.5 border-border-card bg-bg-base text-popover-foreground shadow-md',
        'data-[state=open]:animate-in data-[state=closed]:animate-out',
        'data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0',
        'data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95',
        'data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2',
        position === 'popper' &&
          'data-[side=bottom]:translate-y-1 data-[side=left]:-translate-x-1 data-[side=right]:translate-x-1 data-[side=top]:-translate-y-1',
        className,
      )}
      position={position}
      {...props}
    >
      <SelectScrollUpButton />
      <SelectPrimitive.Viewport
        className={cn(
          'p-1',
          position === 'popper' &&
            'h-[var(--radix-select-trigger-height)] w-full min-w-[var(--radix-select-trigger-width)]',
        )}
      >
        {children}
      </SelectPrimitive.Viewport>
      <SelectScrollDownButton />
    </SelectPrimitive.Content>
  </SelectPrimitive.Portal>
));
SelectContent.displayName = SelectPrimitive.Content.displayName;

const SelectLabel = React.forwardRef<
  React.ElementRef<typeof SelectPrimitive.Label>,
  React.ComponentPropsWithoutRef<typeof SelectPrimitive.Label>
>(({ className, ...props }, ref) => (
  <SelectPrimitive.Label
    ref={ref}
    className={cn('py-1.5 pl-8 pr-2 text-sm font-semibold', className)}
    {...props}
  />
));
SelectLabel.displayName = SelectPrimitive.Label.displayName;

const SelectItem = React.forwardRef<
  React.ElementRef<typeof SelectPrimitive.Item>,
  React.ComponentPropsWithoutRef<typeof SelectPrimitive.Item>
>(({ className, children, ...props }, ref) => (
  <SelectPrimitive.Item
    ref={ref}
    className={cn(
      'relative flex w-full cursor-default select-none items-center rounded-sm py-1.5 pl-2 pr-2 text-sm outline-none text-text-secondary',
      'focus:bg-border-button focus:text-text-primary data-[disabled]:pointer-events-none data-[disabled]:opacity-50',
      className,
    )}
    {...props}
  >
    <span className="absolute right-2 flex h-3.5 w-3.5 items-center justify-center">
      <SelectPrimitive.ItemIndicator>
        <Check className="h-4 w-4" />
      </SelectPrimitive.ItemIndicator>
    </span>
    <SelectPrimitive.ItemText>{children}</SelectPrimitive.ItemText>
  </SelectPrimitive.Item>
));
SelectItem.displayName = SelectPrimitive.Item.displayName;

const SelectSeparator = React.forwardRef<
  React.ElementRef<typeof SelectPrimitive.Separator>,
  React.ComponentPropsWithoutRef<typeof SelectPrimitive.Separator>
>(({ className, ...props }, ref) => (
  <SelectPrimitive.Separator
    ref={ref}
    className={cn('-mx-1 my-1 h-px bg-muted', className)}
    {...props}
  />
));
SelectSeparator.displayName = SelectPrimitive.Separator.displayName;

export {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectScrollDownButton,
  SelectScrollUpButton,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
};

export type RAGFlowSelectOptionType = {
  label: React.ReactNode;
  value: string;
  disabled?: boolean;
  icon?: React.ReactNode;
};

export type RAGFlowSelectGroupOptionType = {
  label: React.ReactNode;
  options: RAGFlowSelectOptionType[];
};

export type RAGFlowSelectProps = Partial<ControllerRenderProps> & {
  FormControlComponent?: typeof FormControl;
  options?: (RAGFlowSelectOptionType | RAGFlowSelectGroupOptionType)[];
  allowClear?: boolean;
  placeholder?: React.ReactNode;
  contentProps?: React.ComponentPropsWithoutRef<typeof SelectPrimitive.Content>;
  triggerClassName?: string;
  onlyShowSelectedIcon?: boolean;
} & SelectPrimitive.SelectProps;

/**
 *
 * Reference:
 * https://github.com/shadcn-ui/ui/discussions/638
 * https://github.com/radix-ui/primitives/discussions/2645#discussioncomment-8343397
 *
 * @export
 * @param {(Partial<ControllerRenderProps> & {
 *   FormControlComponent?: typeof FormControl;
 * })} {
 *   value: initialValue,
 *   onChange,
 *   FormControlComponent,
 * }
 * @return {*}
 */
export const RAGFlowSelect = forwardRef<
  React.ElementRef<typeof SelectPrimitive.Trigger>,
  RAGFlowSelectProps
>(function (
  {
    value: initialValue,
    onChange,
    FormControlComponent,
    options = [],
    allowClear,
    placeholder,
    contentProps = {},
    disabled = false,
    // defaultValue,
    triggerClassName,
    onlyShowSelectedIcon = false,
  },
  ref,
) {
  const [key, setKey] = React.useState(+new Date());
  const [value, setValue] = React.useState<string | undefined>(initialValue);

  const FormControlWidget = FormControlComponent
    ? FormControlComponent
    : ({ children }: React.PropsWithChildren) => <div>{children}</div>;

  const handleChange = useCallback(
    (val?: string) => {
      setValue(val);
      onChange?.(val);
    },
    [onChange],
  );

  const handleReset = useCallback(() => {
    handleChange(undefined);
    setKey(+new Date());
  }, [handleChange]);

  useEffect(() => {
    setValue((preValue) => {
      if (preValue !== initialValue) {
        return initialValue;
      }
      return preValue;
    });
  }, [initialValue]);

  // The value selected in the drop-down box only displays the icon
  const label = React.useMemo(() => {
    let nextOptions = options;
    if (options.some((x) => !('value' in x))) {
      nextOptions = (options as RAGFlowSelectGroupOptionType[]).reduce<
        RAGFlowSelectOptionType[]
      >((pre, cur) => {
        return pre.concat(cur?.options ?? []);
      }, []);
    }

    const option = (nextOptions as RAGFlowSelectOptionType[]).find(
      (x) => x.value === value,
    );

    return onlyShowSelectedIcon ? option?.icon : option?.label;
  }, [onlyShowSelectedIcon, options, value]);

  return (
    <Select
      onValueChange={handleChange}
      value={value}
      key={key}
      disabled={disabled}
    >
      <FormControlWidget>
        <SelectTrigger
          value={value}
          onReset={handleReset}
          allowClear={allowClear}
          ref={ref}
          className={triggerClassName}
        >
          <SelectValue placeholder={placeholder}>{label}</SelectValue>
        </SelectTrigger>
      </FormControlWidget>
      <SelectContent {...contentProps}>
        {options.map((o, idx) => {
          if ('value' in o) {
            return (
              <SelectItem
                value={o.value as RAGFlowSelectOptionType['value']}
                key={o.value}
                disabled={o.disabled}
              >
                <div className="flex items-center gap-1">
                  {o.icon}
                  {o.label}
                </div>
              </SelectItem>
            );
          }

          return (
            <SelectGroup key={idx}>
              <SelectLabel className="pl-2">{o.label}</SelectLabel>
              {o.options.map((x) => (
                <SelectItem value={x.value} key={x.value} disabled={x.disabled}>
                  {x.label}
                </SelectItem>
              ))}
            </SelectGroup>
          );
        })}
      </SelectContent>
    </Select>
  );
});

RAGFlowSelect.displayName = 'RAGFlowSelect';
