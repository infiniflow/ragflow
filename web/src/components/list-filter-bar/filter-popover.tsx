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
import { z, ZodArray, ZodString } from 'zod';

import { Button } from '@/components/ui/button';
import { Input, SearchInput } from '@/components/ui/input';

import { Form, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { t } from 'i18next';
import { FilterField } from './filter-field';
import {
  FilterChange,
  FilterCollection,
  FilterType,
  FilterValue,
} from './interface';

export type CheckboxFormMultipleProps = {
  filters?: FilterCollection[];
  value?: FilterValue;
  onChange?: FilterChange;
  onOpenChange?: (open: boolean) => void;
  setOpen(open: boolean): void;
  filterGroup?: Record<string, string[]>;
};

const filterNestedList = (
  list: FilterType[],
  searchTerm: string,
): FilterType[] => {
  if (!searchTerm) return list;

  const term = searchTerm.toLowerCase();

  return list
    .filter((item) => {
      if (
        item.label.toString().toLowerCase().includes(term) ||
        item.id.toLowerCase().includes(term)
      ) {
        return true;
      }

      if (item.list && item.list.length > 0) {
        const filteredSubList = filterNestedList(item.list, searchTerm);
        return filteredSubList.length > 0;
      }

      return false;
    })
    .map((item) => {
      if (item.list && item.list.length > 0) {
        return {
          ...item,
          list: filterNestedList(item.list, searchTerm),
        };
      }
      return item;
    });
};

function CheckboxFormMultiple({
  filters = [],
  value,
  onChange,
  setOpen,
  filterGroup,
}: CheckboxFormMultipleProps) {
  // const [resolvedFilters, setResolvedFilters] =
  //   useState<FilterCollection[]>(filters);
  const [searchTerms, setSearchTerms] = useState<Record<string, string>>({});

  // useEffect(() => {
  //   if (filters && filters.length > 0) {
  //     setResolvedFilters(filters);
  //   }
  // }, [filters]);

  const fieldsDict = useMemo(() => {
    if (filters.length === 0) {
      return {};
    }

    return filters.reduce<Record<string, any>>((pre, cur) => {
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
  }, [filters]);

  const FormSchema = useMemo(() => {
    if (filters.length === 0) {
      return z.object({});
    }
    return z.object(
      filters.reduce<
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
  }, [filters]);

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: filters.length > 0 ? zodResolver(FormSchema) : undefined,
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
    if (filters.length > 0) {
      form.reset(value || fieldsDict);
    }
  }, [form, value, filters, fieldsDict]);

  const filterList = useMemo(() => {
    const filterSet = filterGroup
      ? Object.values(filterGroup).reduce<Set<string>>((pre, cur) => {
          cur.forEach((item) => pre.add(item));
          return pre;
        }, new Set())
      : new Set();
    return [...filterSet];
  }, [filterGroup]);

  const notInfilterGroup = useMemo(() => {
    return filters.filter((x) => !filterList.includes(x.field));
  }, [filterList, filters]);

  const handleSearchChange = (field: string, value: string) => {
    setSearchTerms((prev) => ({
      ...prev,
      [field]: value,
    }));
  };

  const getFilteredFilters = (originalFilters: FilterCollection[]) => {
    return originalFilters.map((filter) => {
      if (filter.canSearch && searchTerms[filter.field]) {
        const filteredList = filterNestedList(
          filter.list,
          searchTerms[filter.field],
        );
        return { ...filter, list: filteredList };
      }
      return filter;
    });
  };

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        className="space-y-8 px-5 py-2.5 max-h-[80vh] overflow-auto"
        onReset={() => form.reset()}
      >
        <div className="space-y-4">
          {filterGroup &&
            Object.keys(filterGroup).map((key) => {
              const filterKeys = filterGroup[key];
              const originalFilters = filters.filter((x) =>
                filterKeys.includes(x.field),
              );
              const thisFilters = getFilteredFilters(originalFilters);

              return (
                <div
                  key={key}
                  className="flex flex-col space-y-4 border-b border-border-button pb-4"
                >
                  <div className="text-text-primary text-sm">{key}</div>
                  <div className="flex flex-col space-y-4">
                    {thisFilters.map((x) => (
                      <div key={x.field}>
                        {x.canSearch && (
                          <div className="mb-2">
                            <Input
                              placeholder={t('common.search') + '...'}
                              value={searchTerms[x.field] || ''}
                              onChange={(e) =>
                                handleSearchChange(x.field, e.target.value)
                              }
                              className="h-8"
                            />
                          </div>
                        )}
                        <FilterField
                          key={x.field}
                          item={{ ...x, id: x.field }}
                          parent={{
                            ...x,
                            id: x.field,
                            field: ``,
                          }}
                        />
                      </div>
                    ))}
                  </div>
                </div>
              );
            })}
          {notInfilterGroup &&
            notInfilterGroup.map((x) => {
              const filteredItem = getFilteredFilters([x])[0];

              return (
                <FormItem className="space-y-4" key={x.field}>
                  <div>
                    <div className="flex flex-col items-start justify-between mb-2">
                      <FormLabel className="text-text-primary text-sm">
                        {x.label}
                      </FormLabel>
                      {x.canSearch && (
                        <SearchInput
                          placeholder={t('common.search') + '...'}
                          value={searchTerms[x.field] || ''}
                          onChange={(e) =>
                            handleSearchChange(x.field, e.target.value)
                          }
                          className="h-8 w-full"
                        />
                      )}
                    </div>
                  </div>
                  <div className="space-y-4 max-h-[300px] overflow-auto scrollbar-auto">
                    {!!filteredItem.list?.length &&
                      filteredItem.list.map((item) => {
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
                  </div>
                  <FormMessage />
                </FormItem>
              );
            })}
        </div>

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
  filterGroup,
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
          filterGroup={filterGroup}
        ></CheckboxFormMultiple>
      </PopoverContent>
    </Popover>
  );
}
