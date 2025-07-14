import * as React from 'react';

import { cn } from '@/lib/utils';
import { useEffect, useRef } from 'react';
const useAutoResize = (
  textAreaRef: React.RefObject<HTMLTextAreaElement>,
  minRows: number = 3,
  maxRows: number = 6,
) => {
  const isMounted = useRef(false);

  useEffect(() => {
    const getLineHeight = (element: HTMLElement): number => {
      const style = window.getComputedStyle(element);
      return parseInt(style.lineHeight, 10) || 20;
    };

    const adjustHeight = () => {
      if (!textAreaRef.current) return;
      const lineHeight = getLineHeight(textAreaRef.current);
      const maxHeight = maxRows * lineHeight;
      textAreaRef.current.style.height = 'auto';

      requestAnimationFrame(() => {
        if (!textAreaRef.current) return;

        const scrollHeight = textAreaRef.current.scrollHeight;
        textAreaRef.current.style.height = `${Math.min(scrollHeight, maxHeight)}px`;
      });
    };

    const handleResize = () => {
      if (!isMounted.current || !textAreaRef.current) return;

      adjustHeight();
    };

    const initialize = () => {
      if (!textAreaRef.current) return;

      void textAreaRef.current.offsetHeight;

      setTimeout(() => {
        adjustHeight();
        isMounted.current = true;
        textAreaRef.current?.addEventListener('input', adjustHeight);
      }, 50);
    };

    initialize();
    window.addEventListener('resize', handleResize);

    return () => {
      isMounted.current = false;
      textAreaRef.current?.removeEventListener('input', adjustHeight);
      window.removeEventListener('resize', handleResize);
    };
  }, [textAreaRef, minRows, maxRows]);
};
interface TextareaProps
  extends Omit<React.TextareaHTMLAttributes<HTMLTextAreaElement>, 'autoSize'> {
  autoSize?: {
    minRows?: number;
    maxRows?: number;
  };
}
const Textarea = React.forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, autoSize, ...props }, ref) => {
    const textareaRef = useRef<HTMLTextAreaElement>(null);

    useEffect(() => {
      if (typeof ref === 'function') {
        ref(textareaRef.current);
      } else if (ref) {
        ref.current = textareaRef.current;
      }
    }, [ref]);

    useAutoResize(textareaRef, autoSize?.minRows ?? 3, autoSize?.maxRows ?? 10);
    return (
      <textarea
        className={cn(
          'flex min-h-[80px] w-full rounded-md border border-input bg-colors-background-inverse-weak px-3 py-2 text-base ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 md:text-sm overflow-hidden',
          className,
        )}
        rows={autoSize?.minRows ?? undefined}
        style={{
          maxHeight: autoSize?.maxRows
            ? `${autoSize.maxRows * 20}px`
            : undefined,
          overflow: autoSize ? 'auto' : undefined,
        }}
        ref={textareaRef}
        {...props}
      />
    );
  },
);
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
