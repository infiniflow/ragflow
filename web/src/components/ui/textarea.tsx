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
  resize?: 'none' | 'vertical' | 'horizontal' | 'both';
}
const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, autoSize, resize = 'none', ...props }, ref) => {
    const textareaRef = useRef<HTMLTextAreaElement>(null);
    const manualHeightRef = useRef<number | null>(null);
    const isAdjustingRef = useRef(false);
    const getLineHeight = (element: HTMLElement): number => {
      const style = window.getComputedStyle(element);
      return parseInt(style.lineHeight, 10) || 20;
    };
    const adjustHeight = useCallback(() => {
      if (!textareaRef.current || !autoSize) return;
      const lineHeight = getLineHeight(textareaRef.current);
      const maxHeight = (autoSize?.maxRows || 3) * lineHeight;

      isAdjustingRef.current = true;
      textareaRef.current.style.height = 'auto';

      requestAnimationFrame(() => {
        if (!textareaRef.current) return;

        const scrollHeight = textareaRef.current.scrollHeight;
        const desiredHeight = Math.min(scrollHeight, maxHeight);
        const manualHeight = manualHeightRef.current;
        const nextHeight =
          manualHeight && manualHeight > desiredHeight
            ? manualHeight
            : desiredHeight;
        textareaRef.current.style.height = `${nextHeight}px`;
        isAdjustingRef.current = false;
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
    useEffect(() => {
      if (!textareaRef.current || !autoSize || resize === 'none') {
        manualHeightRef.current = null;
        return;
      }
      const element = textareaRef.current;
      let prevHeight = element.getBoundingClientRect().height;
      const observer = new ResizeObserver((entries) => {
        if (isAdjustingRef.current) return;
        const entry = entries[0];
        if (!entry) return;
        const nextHeight = entry.contentRect.height;
        if (Math.abs(nextHeight - prevHeight) > 1) {
          manualHeightRef.current = nextHeight;
        }
        prevHeight = nextHeight;
      });
      observer.observe(element);
      return () => observer.disconnect();
    }, [autoSize, resize]);

    const resizable = resize !== 'none';

    return (
      <textarea
        className={cn(
          'flex min-h-[80px] w-full bg-bg-input rounded-md border border-border-button px-3 py-2 text-base ring-offset-background placeholder:text-text-disabled focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-accent-primary focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 md:text-sm',
          resizable ? 'overflow-auto' : 'overflow-hidden',
          className,
        )}
        rows={autoSize?.minRows ?? props.rows ?? undefined}
        style={{
          maxHeight: autoSize?.maxRows && !resizable
            ? `${autoSize.maxRows * 20}px`
            : undefined,
          resize,
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
