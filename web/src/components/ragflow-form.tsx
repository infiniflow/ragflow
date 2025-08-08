import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { ReactNode, cloneElement, isValidElement } from 'react';
import { ControllerRenderProps, useFormContext } from 'react-hook-form';

type RAGFlowFormItemProps = {
  name: string;
  label: ReactNode;
  tooltip?: ReactNode;
  children: ReactNode | ((field: ControllerRenderProps) => ReactNode);
};

export function RAGFlowFormItem({
  name,
  label,
  tooltip,
  children,
}: RAGFlowFormItemProps) {
  const form = useFormContext();
  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel tooltip={tooltip}>{label}</FormLabel>
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
