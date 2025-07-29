import { cn } from '@/lib/utils';
import { Radio as LucideRadio } from 'lucide-react';
import React, { useContext, useState } from 'react';

const RadioGroupContext = React.createContext<{
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
};

function Radio({ value, checked, disabled, onChange, children }: RadioProps) {
  const groupContext = useContext(RadioGroupContext);
  const isControlled = checked !== undefined;
  // const [internalChecked, setInternalChecked] = useState(false);

  const isChecked = isControlled ? checked : groupContext?.value === value;
  const mergedDisabled = disabled || groupContext?.disabled;

  const handleClick = () => {
    if (mergedDisabled) return;

    // if (!isControlled) {
    //   setInternalChecked(!isChecked);
    // }

    if (onChange) {
      onChange(!isChecked);
    }

    if (groupContext && !groupContext.disabled) {
      groupContext.onChange(value);
    }
  };

  return (
    <label
      className={cn(
        'flex items-center cursor-pointer gap-2 text-sm',
        mergedDisabled && 'cursor-not-allowed opacity-50',
      )}
    >
      <span
        className={cn(
          'flex h-4 w-4 items-center justify-center rounded-full border border-input transition-colors',
          'peer ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2',
          isChecked && 'border-primary bg-primary/10',
          mergedDisabled && 'border-muted',
        )}
        onClick={handleClick}
      >
        {isChecked && (
          <LucideRadio className="h-3 w-3 fill-primary text-primary" />
        )}
      </span>
      {children && <span className="text-foreground">{children}</span>}
    </label>
  );
}

type RadioGroupProps = {
  value?: string | number;
  defaultValue?: string | number;
  onChange?: (value: string | number) => void;
  disabled?: boolean;
  children: React.ReactNode;
  className?: string;
  direction?: 'horizontal' | 'vertical';
};

function Group({
  value,
  defaultValue,
  onChange,
  disabled,
  children,
  className,
  direction = 'horizontal',
}: RadioGroupProps) {
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
        value: mergedValue,
        onChange: handleChange,
        disabled,
      }}
    >
      <div
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
}

const RadioComponent = Object.assign(Radio, {
  Group,
});

export { RadioComponent as Radio };
