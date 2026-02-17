import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { cn } from '@/lib/utils';
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
  className?: string;
} & Pick<UseControllerProps<any>, 'rules'>;

export function RAGFlowFormItem({
  name,
  label,
  tooltip,
  children,
  horizontal = false,
  required = false,
  labelClassName,
  className,
  rules,
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
              'flex items-center w-full space-y-0': horizontal,
            },
            className,
          )}
        >
          {label && (
            <FormLabel
              required={required}
              tooltip={tooltip}
              className={cn({ 'w-1/4': horizontal }, labelClassName)}
            >
              {label}
            </FormLabel>
          )}
          <div
            className={cn('flex flex-col', {
              'w-full': !horizontal,
              'w-3/4': horizontal,
            })}
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
