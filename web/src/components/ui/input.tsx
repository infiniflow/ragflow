import * as React from 'react';

import { cn } from '@/lib/utils';
import { Search } from 'lucide-react';

export interface InputProps
  extends React.InputHTMLAttributes<HTMLInputElement> {
  value?: string | number | readonly string[] | undefined;
}

const Input = React.forwardRef<HTMLInputElement, InputProps>(
  ({ className, type, value, ...props }, ref) => {
    return (
      <input
        type={type}
        className={cn(
          'flex h-8 w-full rounded-md border border-input bg-bg-card px-2 py-2 text-sm ring-offset-background file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50',
          className,
        )}
        ref={ref}
        value={value ?? ''}
        {...props}
      />
    );
  },
);
Input.displayName = 'Input';

export interface ExpandedInputProps extends Omit<InputProps, 'prefix'> {
  prefix?: React.ReactNode;
  suffix?: React.ReactNode;
}

const ExpandedInput = ({
  suffix,
  prefix,
  className,
  ...props
}: ExpandedInputProps) => {
  return (
    <div className="relative">
      <span
        className={cn({
          ['absolute left-3 top-[50%] translate-y-[-50%]']: prefix,
        })}
      >
        {prefix}
      </span>
      <Input
        className={cn({ 'pr-8': !!suffix, 'pl-8': !!prefix }, className)}
        {...props}
      ></Input>
      <span
        className={cn({
          ['absolute right-3 top-[50%] translate-y-[-50%]']: suffix,
        })}
      >
        {suffix}
      </span>
    </div>
  );
};

const SearchInput = (props: InputProps) => {
  return (
    <ExpandedInput
      prefix={<Search className="size-3.5" />}
      {...props}
    ></ExpandedInput>
  );
};

type Value = string | readonly string[] | number | undefined;

export const InnerBlurInput = React.forwardRef<
  HTMLInputElement,
  InputProps & { value: Value; onChange(value: Value): void }
>(({ value, onChange, ...props }, ref) => {
  const [val, setVal] = React.useState<Value>();

  const handleChange: React.ChangeEventHandler<HTMLInputElement> =
    React.useCallback((e) => {
      setVal(e.target.value);
    }, []);

  const handleBlur: React.FocusEventHandler<HTMLInputElement> =
    React.useCallback(
      (e) => {
        onChange?.(e.target.value);
      },
      [onChange],
    );

  React.useEffect(() => {
    setVal(value);
  }, [value]);

  return (
    <Input
      {...props}
      value={val}
      onBlur={handleBlur}
      onChange={handleChange}
      ref={ref}
    ></Input>
  );
});

if (process.env.NODE_ENV !== 'production') {
  InnerBlurInput.whyDidYouRender = true;
}

export const BlurInput = React.memo(InnerBlurInput);

export { ExpandedInput, Input, SearchInput };

type NumberInputProps = { onChange?(value: number): void } & InputProps;

export const NumberInput = React.forwardRef<
  HTMLInputElement,
  NumberInputProps & { value: Value; onChange(value: Value): void }
>(function NumberInput({ onChange, ...props }, ref) {
  return (
    <Input
      type="number"
      onChange={(ev) => {
        const value = ev.target.value;
        onChange?.(value === '' ? 0 : Number(value)); // convert to number
      }}
      {...props}
      ref={ref}
    ></Input>
  );
});
