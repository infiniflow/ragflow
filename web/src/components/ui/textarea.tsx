import * as React from 'react';

import { cn } from '@/lib/utils';

const Textarea = React.forwardRef<
  HTMLTextAreaElement,
  React.ComponentProps<'textarea'>
>(({ className, ...props }, ref) => {
  return (
    <textarea
      className={cn(
        'flex min-h-[80px] w-full rounded-md border border-input bg-colors-background-inverse-weak px-3 py-2 text-base ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 md:text-sm overflow-hidden',
        className,
      )}
      ref={ref}
      {...props}
    />
  );
});
Textarea.displayName = 'Textarea';

export { Textarea };

type Value = string | readonly string[] | number | undefined;

export const BlurTextarea = React.forwardRef<
  HTMLTextAreaElement,
  React.ComponentProps<'textarea'> & {
    value: Value;
    onChange(value: Value): void;
  }
>(({ value, onChange, ...props }, ref) => {
  const [val, setVal] = React.useState<Value>();

  const handleChange: React.ChangeEventHandler<HTMLTextAreaElement> =
    React.useCallback((e) => {
      setVal(e.target.value);
    }, []);

  const handleBlur: React.FocusEventHandler<HTMLTextAreaElement> =
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
    <Textarea
      {...props}
      value={val}
      onBlur={handleBlur}
      onChange={handleChange}
      ref={ref}
    ></Textarea>
  );
});
