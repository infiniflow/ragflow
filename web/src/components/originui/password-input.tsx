// https://originui.com/r/comp-23.json

'use client';

import { EyeIcon, EyeOffIcon } from 'lucide-react';
import React, { useId, useState } from 'react';
import { Input, InputProps } from '../ui/input';

export default React.forwardRef<HTMLInputElement, InputProps>(
  function PasswordInput({ ...props }, ref) {
    const id = useId();
    const [isVisible, setIsVisible] = useState<boolean>(false);

    const toggleVisibility = () => setIsVisible((prevState) => !prevState);

    return (
      <div className="*:not-first:mt-2">
        {/* <Label htmlFor={id}>Show/hide password input</Label> */}
        <div className="relative">
          <Input
            id={id}
            className="pe-9"
            placeholder="Password"
            type={isVisible ? 'text' : 'password'}
            ref={ref}
            {...props}
          />
          <button
            className="text-muted-foreground/80 hover:text-foreground focus-visible:border-ring focus-visible:ring-ring/50 absolute inset-y-0 end-0 flex h-full w-9 items-center justify-center rounded-e-md transition-[color,box-shadow] outline-none focus:z-10 focus-visible:ring-[3px] disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50"
            type="button"
            onClick={toggleVisibility}
            aria-label={isVisible ? 'Hide password' : 'Show password'}
            aria-pressed={isVisible}
            aria-controls="password"
          >
            {isVisible ? (
              <EyeOffIcon size={16} aria-hidden="true" />
            ) : (
              <EyeIcon size={16} aria-hidden="true" />
            )}
          </button>
        </div>
      </div>
    );
  },
);
