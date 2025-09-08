import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { cn } from '@/lib/utils';
import { ReactNode, cloneElement, isValidElement } from 'react';
import { ControllerRenderProps, useFormContext } from 'react-hook-form';

type RAGFlowFormItemProps = {
  name: string;
  label: ReactNode;
  tooltip?: ReactNode;
  children: ReactNode | ((field: ControllerRenderProps) => ReactNode);
  horizontal?: boolean;
};

export function RAGFlowFormItem({
  name,
  label,
  tooltip,
  children,
  horizontal = false,
}: RAGFlowFormItemProps) {
  const form = useFormContext();
  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem
          className={cn({
            'flex items-center': horizontal,
          })}
        >
          <FormLabel tooltip={tooltip} className={cn({ 'w-1/4': horizontal })}>
            {label}
          </FormLabel>
          <FormControl>
            {typeof children === 'function'
              ? children(field)
              : isValidElement(children)
                ? cloneElement(children, { ...field })
                : children}
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
