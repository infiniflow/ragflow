'use client';

import * as SliderPrimitive from '@radix-ui/react-slider';
import * as React from 'react';

import { cn } from '@/lib/utils';

const Slider = React.forwardRef<
  React.ElementRef<typeof SliderPrimitive.Root>,
  React.ComponentPropsWithoutRef<typeof SliderPrimitive.Root>
>(({ className, ...props }, ref) => (
  <SliderPrimitive.Root
    ref={ref}
    className={cn(
      'relative flex w-full touch-none select-none items-center',
      className,
    )}
    {...props}
  >
    <SliderPrimitive.Track className="relative h-1 w-full grow overflow-hidden rounded-full bg-border-button">
      <SliderPrimitive.Range className="absolute h-full bg-accent-primary" />
    </SliderPrimitive.Track>

    <SliderPrimitive.Thumb
      className="
      block h-2.5 w-2.5 rounded-full border-2 border-accent-primary bg-white ring-offset-background transition-colors
      focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-foreground focus-visible:ring-offset-2
      disabled:pointer-events-none disabled:opacity-50 cursor-pointer"
    />
  </SliderPrimitive.Root>
));
Slider.displayName = SliderPrimitive.Root.displayName;

type SliderProps = Omit<
  React.ComponentPropsWithoutRef<typeof SliderPrimitive.Root>,
  'onChange' | 'value'
> & { onChange: (value: number) => void; value: number };

const FormSlider = React.forwardRef<
  React.ElementRef<typeof SliderPrimitive.Root>,
  SliderProps
>(function FormSlider({ onChange, value, ...props }, ref) {
  return (
    <Slider
      ref={ref}
      {...props}
      value={[value]}
      onValueChange={(vals) => {
        onChange(vals[0]);
      }}
    />
  );
});

Slider.displayName = SliderPrimitive.Root.displayName;

export { FormSlider, Slider };
