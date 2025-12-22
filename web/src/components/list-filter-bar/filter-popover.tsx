import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { zodResolver } from '@hookform/resolvers/zod';
import {
  PropsWithChildren,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from 'react';
import { useForm } from 'react-hook-form';
import { ZodArray, ZodString, z } from 'zod';

import { Button } from '@/components/ui/button';

import {
  Form,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { t } from 'i18next';
import { FilterField } from './filter-field';
import { FilterChange, FilterCollection, FilterValue } from './interface';

export type CheckboxFormMultipleProps = {
  filters?: FilterCollection[];
  value?: FilterValue;
  onChange?: FilterChange;
  onOpenChange?: (open: boolean) => void;
  setOpen(open: boolean): void;
};

function CheckboxFormMultiple({
  filters = [],
  value,
  onChange,
  setOpen,
}: CheckboxFormMultipleProps) {
  const [resolvedFilters, setResolvedFilters] =
    useState<FilterCollection[]>(filters);

  useEffect(() => {
    if (filters && filters.length > 0) {
      setResolvedFilters(filters);
    }
  }, [filters]);

  const fieldsDict = useMemo(() => {
    if (resolvedFilters.length === 0) {
      return {};
    }

    return resolvedFilters.reduce<Record<string, any>>((pre, cur) => {
      const hasNested = cur.list?.some(
        (item) => item.list && item.list.length > 0,
      );

      if (hasNested) {
        pre[cur.field] = {};
      } else {
        pre[cur.field] = [];
      }
      return pre;
    }, {});
  }, [resolvedFilters]);

  const FormSchema = useMemo(() => {
    if (resolvedFilters.length === 0) {
      return z.object({});
    }

    return z.object(
      resolvedFilters.reduce<
        Record<
          string,
          ZodArray<ZodString, 'many'> | z.ZodObject<any> | z.ZodOptional<any>
        >
      >((pre, cur) => {
        const hasNested = cur.list?.some(
          (item) => item.list && item.list.length > 0,
        );

        if (hasNested) {
          pre[cur.field] = z
            .record(z.string(), z.array(z.string().optional()).optional())
            .optional();
        } else {
          pre[cur.field] = z.array(z.string().optional()).optional();
        }

        return pre;
      }, {}),
    );
  }, [resolvedFilters]);

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: resolvedFilters.length > 0 ? zodResolver(FormSchema) : undefined,
    defaultValues: fieldsDict,
  });

  function onSubmit() {
    const formValues = form.getValues();
    onChange?.({ ...formValues });
    setOpen(false);
  }

  const onReset = useCallback(() => {
    onChange?.(fieldsDict);
    setOpen(false);
  }, [fieldsDict, onChange, setOpen]);

  useEffect(() => {
    if (resolvedFilters.length > 0) {
      form.reset(value || fieldsDict);
    }
  }, [form, value, resolvedFilters, fieldsDict]);

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        className="space-y-8 px-5 py-2.5"
        onReset={() => form.reset()}
      >
        {filters.map((x) => (
          <FormField
            key={x.field}
            control={form.control}
            name={x.field}
            render={() => (
              <FormItem className="space-y-4">
                <div>
                  <FormLabel className="text-text-primary">{x.label}</FormLabel>
                </div>
                {x.list.map((item) => {
                  return (
                    <FilterField
                      key={item.id}
                      item={{ ...item }}
                      parent={{
                        ...x,
                        id: x.field,
                        // field: `${x.field}${item.field ? '.' + item.field : ''}`,
                      }}
                    />
                  );
                })}
                <FormMessage />
              </FormItem>
            )}
          />
        ))}
        <div className="flex justify-end gap-5">
          <Button
            type="button"
            variant={'outline'}
            size={'sm'}
            onClick={onReset}
          >
            {t('common.clear')}
          </Button>
          <Button
            type="submit"
            onClick={() => {
              console.log(form.formState.errors, form.getValues());
            }}
            size={'sm'}
          >
            {t('common.submit')}
          </Button>
        </div>
      </form>
    </Form>
  );
}

export function FilterPopover({
  children,
  value,
  onChange,
  onOpenChange,
  filters,
}: PropsWithChildren & Omit<CheckboxFormMultipleProps, 'setOpen'>) {
  const [open, setOpen] = useState(false);
  const onOpenChangeFun = useCallback(
    (e: boolean) => {
      onOpenChange?.(e);
      setOpen(e);
    },
    [onOpenChange],
  );
  return (
    <Popover open={open} onOpenChange={onOpenChangeFun}>
      <PopoverTrigger asChild>{children}</PopoverTrigger>
      <PopoverContent className="p-0">
        <CheckboxFormMultiple
          onChange={onChange}
          value={value}
          filters={filters}
          setOpen={setOpen}
        ></CheckboxFormMultiple>
      </PopoverContent>
    </Popover>
  );
}
