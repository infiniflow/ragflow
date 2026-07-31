import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { cn } from '@/lib/utils';
import { ArrowUpRight } from 'lucide-react';
import { ReactNode, cloneElement, isValidElement } from 'react';
import {
  ControllerRenderProps,
  UseControllerProps,
  useFormContext,
} from 'react-hook-form';

type RAGFlowFormItemProps = {
  name: string;
  label?: ReactNode;
  tooltip?: ReactNode;
  children: ReactNode | ((field: ControllerRenderProps) => ReactNode);
  horizontal?: boolean;
  required?: boolean;
  labelClassName?: string;
  valueClassName?: string;
  className?: string;
  labelLink?: {
    text: ReactNode;
    onClick: () => void;
  };
} & Pick<UseControllerProps<any>, 'rules'>;

export function RAGFlowFormItem({
  name,
  label,
  tooltip,
  children,
  horizontal = false,
  required = false,
  labelClassName,
  valueClassName,
  className,
  rules,
  labelLink,
}: RAGFlowFormItemProps) {
  const form = useFormContext();
  return (
    <FormField
      control={form.control}
      rules={rules}
      name={name}
      render={({ field }) => (
        <FormItem
          className={cn(
            {
              'flex items-center w-full space-y-0 justify-between': horizontal,
            },
            className,
          )}
        >
          {label && (
            <div className={cn('flex items-center gap-2 justify-between')}>
              <FormLabel
                required={required}
                tooltip={tooltip}
                className={cn(labelClassName)}
              >
                {label}
              </FormLabel>
              {labelLink && (
                <div
                  className="text-sm flex text-text-primary cursor-pointer items-center shrink-0"
                  onClick={labelLink.onClick}
                >
                  {labelLink.text}
                  <ArrowUpRight size={14} />
                </div>
              )}
            </div>
          )}
          <div
            className={cn(
              'flex flex-col',
              {
                'w-full': !horizontal,
                'w-3/4': horizontal,
              },
              valueClassName,
            )}
          >
            <FormControl>
              {typeof children === 'function'
                ? children(field)
                : isValidElement(children)
                  ? cloneElement(children, { ...field })
                  : children}
            </FormControl>
            <FormMessage />
          </div>
        </FormItem>
      )}
    />
  );
}
