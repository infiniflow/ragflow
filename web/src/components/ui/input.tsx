import * as React from 'react';

import { cn } from '@/lib/utils';
import { Search } from 'lucide-react';

export interface InputProps
  extends React.InputHTMLAttributes<HTMLInputElement> {}

const Input = React.forwardRef<HTMLInputElement, InputProps>(
  ({ className, type, ...props }, ref) => {
    return (
      <input
        type={type}
        className={cn(
          'flex h-10 w-full rounded-md border border-input bg-colors-background-inverse-weak px-3 py-2 text-sm ring-offset-background file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50',
          className,
        )}
        ref={ref}
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

const ExpandedInput = ({ suffix, prefix, ...props }: ExpandedInputProps) => {
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
        className={cn({ 'pr-10': suffix, 'pl-10': prefix })}
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
  return <ExpandedInput suffix={<Search />} {...props}></ExpandedInput>;
};

export { ExpandedInput, Input, SearchInput };
