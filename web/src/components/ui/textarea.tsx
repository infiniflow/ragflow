import { cn } from '@/lib/utils';
import {
  ChangeEventHandler,
  ComponentProps,
  FocusEventHandler,
  forwardRef,
  TextareaHTMLAttributes,
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react';
interface TextareaProps
  extends Omit<TextareaHTMLAttributes<HTMLTextAreaElement>, 'autoSize'> {
  autoSize?: {
    minRows?: number;
    maxRows?: number;
  };
}
const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, autoSize, ...props }, ref) => {
    const textareaRef = useRef<HTMLTextAreaElement>(null);
    const getLineHeight = (element: HTMLElement): number => {
      const style = window.getComputedStyle(element);
      return parseInt(style.lineHeight, 10) || 20;
    };
    const adjustHeight = useCallback(() => {
      if (!textareaRef.current) return;
      const lineHeight = getLineHeight(textareaRef.current);
      const maxHeight = (autoSize?.maxRows || 3) * lineHeight;
      textareaRef.current.style.height = 'auto';

      requestAnimationFrame(() => {
        if (!textareaRef.current) return;

        const scrollHeight = textareaRef.current.scrollHeight;
        textareaRef.current.style.height = `${Math.min(scrollHeight, maxHeight)}px`;
      });
    }, [autoSize]);

    useEffect(() => {
      if (autoSize) {
        adjustHeight();
      }
    }, [textareaRef, autoSize, adjustHeight]);

    useEffect(() => {
      if (typeof ref === 'function') {
        ref(textareaRef.current);
      } else if (ref) {
        ref.current = textareaRef.current;
      }
    }, [ref]);
    return (
      <textarea
        className={cn(
          'flex min-h-[80px] w-full bg-bg-input rounded-md border border-input px-3 py-2 text-base ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 md:text-sm overflow-hidden',
          className,
        )}
        rows={autoSize?.minRows ?? props.rows ?? undefined}
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

export const BlurTextarea = forwardRef<
  HTMLTextAreaElement,
  ComponentProps<'textarea'> & {
    value: Value;
    onChange(value: Value): void;
  }
>(({ value, onChange, ...props }, ref) => {
  const [val, setVal] = useState<Value>();

  const handleChange: ChangeEventHandler<HTMLTextAreaElement> = useCallback(
    (e) => {
      setVal(e.target.value);
    },
    [],
  );

  const handleBlur: FocusEventHandler<HTMLTextAreaElement> = useCallback(
    (e) => {
      onChange?.(e.target.value);
    },
    [onChange],
  );

  useEffect(() => {
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
