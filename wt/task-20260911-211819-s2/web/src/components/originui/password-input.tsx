// https://originui.com/r/comp-23.json

'use client';

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
        </div>
      </div>
    );
  },
);
