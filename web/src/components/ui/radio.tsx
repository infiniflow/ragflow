import { cn } from '@/lib/utils';
import React, { useContext, useState } from 'react';

const RadioGroupContext = React.createContext<{
  name?: string;
  value: string | number;
  onChange: (value: string | number) => void;
  disabled?: boolean;
} | null>(null);

type RadioProps = {
  value: string | number;
  checked?: boolean;
  disabled?: boolean;
  onChange?: (checked: boolean) => void;
  children?: React.ReactNode;
} & Omit<
  React.InputHTMLAttributes<HTMLInputElement>,
  'value' | 'checked' | 'onChange'
>;

function Radio({
  className,
  value,
  checked,
  disabled,
  onChange,
  children,
  ...props
}: RadioProps) {
  const groupContext = useContext(RadioGroupContext);
  const isControlled = checked !== undefined;
  // const [internalChecked, setInternalChecked] = useState(false);

  const isChecked = isControlled ? checked : groupContext?.value === value;
  const mergedDisabled = disabled || groupContext?.disabled;

  const handleChange = (nextChecked: boolean) => {
    if (mergedDisabled) return;

    if (onChange) {
      onChange(nextChecked);
    }

    if (nextChecked && groupContext && !groupContext.disabled) {
      groupContext.onChange(value);
    }
  };

  return (
    <label
      className={cn(
        'group/radio relative flex items-center cursor-pointer gap-2 text-sm',
        mergedDisabled && 'cursor-not-allowed opacity-50',
      )}
    >
      <input
        type="radio"
        value={value}
        checked={isChecked}
        onChange={(e) => handleChange(e.target.checked)}
        disabled={mergedDisabled}
        className={cn('peer absolute size-[1px] opacity-0', className)}
        {...props}
        name={groupContext?.name}
      />

      <div
        className={cn(
          'flex h-4 w-4 items-center justify-center rounded-full text-border-button border border-current transition-colors',
          'group-hover/radio:text-border-default hover:text-border-default',
          'peer-focus:text-text-primary',
          isChecked && 'border-primary bg-primary/10',
          mergedDisabled && 'border-muted',
        )}
      >
        <div
          className={cn(
            'h-2 w-2 fill-primary text-primary bg-text-primary rounded-full opacity-0 scale-0 transition-all',
            isChecked && 'opacity-100 scale-100',
          )}
        />
      </div>

      {children && <span className="text-foreground">{children}</span>}
    </label>
  );
}

type RadioGroupProps = {
  name?: string;
  value?: string | number;
  defaultValue?: string | number;
  onChange?: (value: string | number) => void;
  disabled?: boolean;
  children: React.ReactNode;
  className?: string;
  direction?: 'horizontal' | 'vertical';
};

const Group = React.forwardRef<HTMLDivElement, RadioGroupProps>(
  (
    {
      name,
      value,
      defaultValue,
      onChange,
      disabled,
      children,
      className,
      direction = 'horizontal',
    },
    ref,
  ) => {
    const [internalValue, setInternalValue] = useState(defaultValue || '');

    const isControlled = value !== undefined;
    const mergedValue = isControlled ? value : internalValue;

    const handleChange = (val: string | number) => {
      if (disabled) return;

      if (!isControlled) {
        setInternalValue(val);
      }

      if (onChange) {
        onChange(val);
      }
    };

    return (
      <RadioGroupContext.Provider
        value={{
          name,
          value: mergedValue,
          onChange: handleChange,
          disabled,
        }}
      >
        <div
          ref={ref}
          className={cn(
            'flex gap-4',
            direction === 'vertical' ? 'flex-col' : 'flex-row',
            className,
          )}
        >
          {React.Children.map(children, (child) =>
            React.cloneElement(child as React.ReactElement, {
              disabled: disabled || child?.props?.disabled,
            }),
          )}
        </div>
      </RadioGroupContext.Provider>
    );
  },
);

const RadioComponent = Object.assign(Radio, {
  Group,
});

Group.displayName = 'RadioGroup';
export { RadioComponent as Radio };
