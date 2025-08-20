import * as React from 'react';

import { cn } from '@/lib/utils';
type InputProps = React.ComponentProps<'input'> & {
  icon?: React.ReactNode;
  iconPosition?: 'left' | 'right';
};

const Input = function ({
  className,
  type,
  icon,
  iconPosition = 'left',
  ref,
  ...props
}: InputProps) {
  return (
    <div className="relative">
      {icon && (
        <div
          className={cn(
            'absolute w-1 top-0 flex h-full items-center justify-center pointer-events-none',
            iconPosition === 'left' ? 'left-5' : 'right-5',
            iconPosition === 'left' ? 'pr-2' : 'pl-2',
          )}
        >
          {icon}
        </div>
      )}
      <input
        ref={ref}
        type={type}
        data-slot="input"
        className={cn(
          'border-input file:text-foreground placeholder:text-muted-foreground/70 flex h-9 w-full min-w-0 rounded-md border bg-transparent px-3 py-1 text-sm shadow-xs transition-[color,box-shadow] outline-none file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50',
          'focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px]',
          'aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 aria-invalid:border-destructive',
          type === 'search' &&
            '[&::-webkit-search-cancel-button]:appearance-none [&::-webkit-search-decoration]:appearance-none [&::-webkit-search-results-button]:appearance-none [&::-webkit-search-results-decoration]:appearance-none',
          type === 'file' &&
            'text-muted-foreground/70 file:border-input file:text-foreground p-0 pr-3 italic file:me-3 file:h-full file:border-0 file:border-r file:border-solid file:bg-transparent file:px-3 file:text-sm file:font-medium file:not-italic',
          icon && iconPosition === 'left' && 'pl-7',
          icon && iconPosition === 'right' && 'pr-7',
          className,
        )}
        {...props}
      />
    </div>
  );
};

export { Input };
export default React.forwardRef(Input);
