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

    const newValue = {
      ...currentValue,
      [parentId]: newParentValues,
    };

    if (newValue[parentId].length === 0) {
      delete newValue[parentId];
    }

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
        className={`flex items-center justify-between text-text-primary text-xs ${level > 0 ? 'ml-4' : ''}`}
      >
        <FormItem className="flex flex-row space-x-3 space-y-0 items-center">
          <FormControl>
            <Checkbox
              checked={field.value?.includes(item.id.toString())}
              onCheckedChange={(checked: boolean) =>
                handleCheckChange({ checked, field, item })
              }
            />
          </FormControl>
          <FormLabel onClick={(e) => e.stopPropagation()}>
            {item.label}
          </FormLabel>
        </FormItem>
        {item.count !== undefined && (
          <span className="text-sm">{item.count}</span>
        )}
      </div>
    );
  },
);

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
        name={parent.field as string}
        render={({ field }) => {
          if (hasNestedList) {
            return (
              <div className={`flex flex-col gap-2 ${level > 0 ? 'ml-4' : ''}`}>
                <div
                  className="flex items-center justify-between cursor-pointer"
                  onClick={() => {
                    setShowAll(!showAll);
                  }}
                >
                  <FormLabel className="text-text-primary">
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
                    // <FilterItem key={child.id} item={child} field={child.field} level={level+1} />
                    // <div
                    //   className="flex flex-row space-x-3 space-y-0 items-center"
                    //   key={child.id}
                    // >
                    //   <FormControl>
                    //     <Checkbox
                    //       checked={field.value?.includes(child.id.toString())}
                    //       onCheckedChange={(checked) =>
                    //         handleCheckChange({ checked, field, item: child })
                    //       }
                    //     />
                    //   </FormControl>
                    //   <FormLabel onClick={(e) => e.stopPropagation()}>
                    //     {child.label}
                    //   </FormLabel>
                    // </div>
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
