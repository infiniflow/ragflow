import { Checkbox } from '@/components/ui/checkbox';
import { ChevronDown, ChevronUp } from 'lucide-react';
import { memo, useState } from 'react';
import { ControllerRenderProps, useFormContext } from 'react-hook-form';
import { FormControl, FormField, FormItem, FormLabel } from '../ui/form';
import { FilterType } from './interface';
const handleCheckChange = ({
  checked,
  field,
  item,
  isNestedField = false,
  parentId = '',
}: {
  checked: boolean;
  field: ControllerRenderProps<
    { [x: string]: { [x: string]: any } | string[] },
    string
  >;
  item: FilterType;
  isNestedField?: boolean;
  parentId?: string;
}) => {
  if (isNestedField && parentId) {
    const currentValue = field.value || {};
    const currentParentValues =
      (currentValue as Record<string, string[]>)[parentId] || [];

    const newParentValues = checked
      ? [...currentParentValues, item.id.toString()]
      : currentParentValues.filter(
          (value: string) => value !== item.id.toString(),
        );

    const newValue = newParentValues?.length
      ? {
          ...currentValue,
          [parentId]: newParentValues,
        }
      : { ...currentValue };

    // if (newValue[parentId].length === 0) {
    //   delete newValue[parentId];
    // }

    return field.onChange(newValue);
  } else {
    const list = checked
      ? [...(Array.isArray(field.value) ? field.value : []), item.id.toString()]
      : (Array.isArray(field.value) ? field.value : []).filter(
          (value) => value !== item.id.toString(),
        );
    return field.onChange(list);
  }
};

const FilterItem = memo(
  ({
    item,
    field,
    level = 0,
  }: {
    item: FilterType;
    field: ControllerRenderProps<
      { [x: string]: { [x: string]: any } | string[] },
      string
    >;
    level: number;
  }) => {
    return (
      <div
        className={`flex items-center justify-between text-text-primary text-xs ${level > 0 ? 'ml-1' : ''}`}
      >
        <FormItem className="flex flex-row space-x-3 space-y-0 items-center ">
          <FormControl>
            <div className="flex space-x-3">
              <Checkbox
                checked={field.value?.includes(item.id.toString())}
                onCheckedChange={(checked: boolean) =>
                  handleCheckChange({ checked, field, item })
                }
                // className="hidden group-hover:block"
              />
              <div
                onClick={() =>
                  handleCheckChange({
                    checked: !field.value?.includes(item.id.toString()),
                    field,
                    item,
                  })
                }
                className="truncate w-[200px] text-sm font-normal leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70 text-text-secondary"
              >
                {item.label}
              </div>
            </div>
          </FormControl>
        </FormItem>
        {item.count !== undefined && (
          <span className="text-sm">{item.count}</span>
        )}
      </div>
    );
  },
);
FilterItem.displayName = 'FilterItem';
export const FilterField = memo(
  ({
    item,
    parent,
    level = 0,
  }: {
    item: FilterType;
    parent: FilterType;
    level?: number;
  }) => {
    const form = useFormContext();
    const [showAll, setShowAll] = useState(false);
    const hasNestedList = item.list && item.list.length > 0;

    return (
      <FormField
        key={item.id}
        control={form.control}
        name={parent.field?.toString() as string}
        render={({ field }) => {
          if (hasNestedList) {
            return (
              <div className={`flex flex-col gap-2 ${level > 0 ? 'ml-1' : ''}`}>
                <div
                  className="flex items-center justify-between cursor-pointer"
                  onClick={() => {
                    setShowAll(!showAll);
                  }}
                >
                  <FormLabel className="text-text-secondary text-sm">
                    {item.label}
                  </FormLabel>
                  {showAll ? (
                    <ChevronUp size={12} />
                  ) : (
                    <ChevronDown size={12} />
                  )}
                </div>
                {showAll &&
                  item.list?.map((child) => (
                    <FilterField
                      key={child.id}
                      item={child}
                      parent={{
                        ...item,
                        field: `${parent.field}.${item.field}`,
                      }}
                      level={level + 1}
                    />
                  ))}
              </div>
            );
          }

          return <FilterItem item={item} field={field} level={level} />;
        }}
      />
    );
  },
);

FilterField.displayName = 'FilterField';
