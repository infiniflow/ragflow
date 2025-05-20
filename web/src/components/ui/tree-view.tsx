'use client';

import { cn } from '@/lib/utils';
import * as AccordionPrimitive from '@radix-ui/react-accordion';
import { cva } from 'class-variance-authority';
import { ChevronRight } from 'lucide-react';
import React from 'react';

const treeVariants = cva(
  'group hover:before:opacity-100 before:absolute before:rounded-lg before:left-0 px-2 before:w-full before:opacity-0 before:bg-accent/70 before:h-[2rem] before:-z-10',
);

const selectedTreeVariants = cva(
  'before:opacity-100 before:bg-[#4E74Fd]/70 text-accent-foreground',
);

export interface TreeDataItem {
  id: string;
  name: string;
  icon?: any;
  selectedIcon?: any;
  openIcon?: any;
  children?: TreeDataItem[];
  actions?: React.ReactNode;
  onClick?: () => void;
}

type TreeProps = React.HTMLAttributes<HTMLDivElement> & {
  data: TreeDataItem[] | TreeDataItem;
  initialSelectedItemId?: string;
  onSelectChange?: (item: TreeDataItem | undefined) => void;
  expandAll?: boolean;
  defaultNodeIcon?: any;
  defaultLeafIcon?: any;
};

const TreeView = React.forwardRef<HTMLDivElement, TreeProps>(
  (
    {
      data,
      initialSelectedItemId,
      onSelectChange,
      expandAll,
      defaultLeafIcon,
      defaultNodeIcon,
      className,
      ...props
    },
    ref,
  ) => {
    const [selectedItemId, setSelectedItemId] = React.useState<
      string | undefined
    >(initialSelectedItemId);

    const handleSelectChange = React.useCallback(
      (item: TreeDataItem | undefined) => {
        setSelectedItemId(item?.id);
        if (onSelectChange) {
          onSelectChange(item);
        }
      },
      [onSelectChange],
    );

    const expandedItemIds = React.useMemo(() => {
      if (!initialSelectedItemId) {
        return [] as string[];
      }

      const ids: string[] = [];

      function walkTreeItems(
        items: TreeDataItem[] | TreeDataItem,
        targetId: string,
      ) {
        if (items instanceof Array) {
          for (let i = 0; i < items.length; i++) {
            ids.push(items[i]!.id);
            if (walkTreeItems(items[i]!, targetId) && !expandAll) {
              return true;
            }
            if (!expandAll) ids.pop();
          }
        } else if (!expandAll && items.id === targetId) {
          return true;
        } else if (items.children) {
          return walkTreeItems(items.children, targetId);
        }
      }

      walkTreeItems(data, initialSelectedItemId);
      return ids;
    }, [data, expandAll, initialSelectedItemId]);

    return (
      <div className={cn('overflow-hidden relative p-2', className)}>
        <TreeItem
          data={data}
          ref={ref}
          selectedItemId={selectedItemId}
          handleSelectChange={handleSelectChange}
          expandedItemIds={expandedItemIds}
          defaultLeafIcon={defaultLeafIcon}
          defaultNodeIcon={defaultNodeIcon}
          {...props}
        />
      </div>
    );
  },
);
TreeView.displayName = 'TreeView';

type TreeItemProps = TreeProps & {
  selectedItemId?: string;
  handleSelectChange: (item: TreeDataItem | undefined) => void;
  expandedItemIds: string[];
  defaultNodeIcon?: any;
  defaultLeafIcon?: any;
};

const TreeItem = React.forwardRef<HTMLDivElement, TreeItemProps>(
  (
    {
      className,
      data,
      selectedItemId,
      handleSelectChange,
      expandedItemIds,
      defaultNodeIcon,
      defaultLeafIcon,
      ...props
    },
    ref,
  ) => {
    if (!(data instanceof Array)) {
      data = [data];
    }
    return (
      <div ref={ref} role="tree" className={className} {...props}>
        <ul>
          {data.map((item) => (
            <li key={item.id}>
              {item.children ? (
                <TreeNode
                  item={item}
                  selectedItemId={selectedItemId}
                  expandedItemIds={expandedItemIds}
                  handleSelectChange={handleSelectChange}
                  defaultNodeIcon={defaultNodeIcon}
                  defaultLeafIcon={defaultLeafIcon}
                />
              ) : (
                <TreeLeaf
                  item={item}
                  selectedItemId={selectedItemId}
                  handleSelectChange={handleSelectChange}
                  defaultLeafIcon={defaultLeafIcon}
                />
              )}
            </li>
          ))}
        </ul>
      </div>
    );
  },
);
TreeItem.displayName = 'TreeItem';

const TreeNode = ({
  item,
  handleSelectChange,
  expandedItemIds,
  selectedItemId,
  defaultNodeIcon,
  defaultLeafIcon,
}: {
  item: TreeDataItem;
  handleSelectChange: (item: TreeDataItem | undefined) => void;
  expandedItemIds: string[];
  selectedItemId?: string;
  defaultNodeIcon?: any;
  defaultLeafIcon?: any;
}) => {
  const [value, setValue] = React.useState(
    expandedItemIds.includes(item.id) ? [item.id] : [],
  );
  return (
    <AccordionPrimitive.Root
      type="multiple"
      value={value}
      onValueChange={(s) => setValue(s)}
    >
      <AccordionPrimitive.Item value={item.id}>
        <AccordionTrigger
          className={cn(
            treeVariants(),
            selectedItemId === item.id && selectedTreeVariants(),
          )}
          onClick={() => {
            handleSelectChange(item);
            item.onClick?.();
          }}
        >
          <TreeIcon
            item={item}
            isSelected={selectedItemId === item.id}
            isOpen={value.includes(item.id)}
            default={defaultNodeIcon}
          />
          <span className="text-sm truncate">{item.name}</span>
          <TreeActions isSelected={selectedItemId === item.id}>
            {item.actions}
          </TreeActions>
        </AccordionTrigger>
        <AccordionContent className="ml-4 pl-1 border-l">
          <TreeItem
            data={item.children ? item.children : item}
            selectedItemId={selectedItemId}
            handleSelectChange={handleSelectChange}
            expandedItemIds={expandedItemIds}
            defaultLeafIcon={defaultLeafIcon}
            defaultNodeIcon={defaultNodeIcon}
          />
        </AccordionContent>
      </AccordionPrimitive.Item>
    </AccordionPrimitive.Root>
  );
};

const TreeLeaf = React.forwardRef<
  HTMLDivElement,
  React.HTMLAttributes<HTMLDivElement> & {
    item: TreeDataItem;
    selectedItemId?: string;
    handleSelectChange: (item: TreeDataItem | undefined) => void;
    defaultLeafIcon?: any;
  }
>(
  (
    {
      className,
      item,
      selectedItemId,
      handleSelectChange,
      defaultLeafIcon,
      ...props
    },
    ref,
  ) => {
    return (
      <div
        ref={ref}
        className={cn(
          'ml-5 flex text-left items-center py-2 cursor-pointer before:right-1',
          treeVariants(),
          className,
          selectedItemId === item.id && selectedTreeVariants(),
        )}
        onClick={() => {
          handleSelectChange(item);
          item.onClick?.();
        }}
        {...props}
      >
        <TreeIcon
          item={item}
          isSelected={selectedItemId === item.id}
          default={defaultLeafIcon}
        />
        <span className="flex-grow text-sm truncate">{item.name}</span>
        <TreeActions isSelected={selectedItemId === item.id}>
          {item.actions}
        </TreeActions>
      </div>
    );
  },
);
TreeLeaf.displayName = 'TreeLeaf';

const AccordionTrigger = React.forwardRef<
  React.ElementRef<typeof AccordionPrimitive.Trigger>,
  React.ComponentPropsWithoutRef<typeof AccordionPrimitive.Trigger>
>(({ className, children, ...props }, ref) => (
  <AccordionPrimitive.Header>
    <AccordionPrimitive.Trigger
      ref={ref}
      className={cn(
        'flex flex-1 w-full items-center py-2 transition-all first:[&[data-state=open]>svg]:rotate-90',
        className,
      )}
      {...props}
    >
      <ChevronRight className="h-4 w-4 shrink-0 transition-transform duration-200 text-accent-foreground/50 mr-1" />
      {children}
    </AccordionPrimitive.Trigger>
  </AccordionPrimitive.Header>
));
AccordionTrigger.displayName = AccordionPrimitive.Trigger.displayName;

const AccordionContent = React.forwardRef<
  React.ElementRef<typeof AccordionPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof AccordionPrimitive.Content>
>(({ className, children, ...props }, ref) => (
  <AccordionPrimitive.Content
    ref={ref}
    className={cn(
      'overflow-hidden text-sm transition-all data-[state=closed]:animate-accordion-up data-[state=open]:animate-accordion-down',
      className,
    )}
    {...props}
  >
    <div className="pb-1 pt-0">{children}</div>
  </AccordionPrimitive.Content>
));
AccordionContent.displayName = AccordionPrimitive.Content.displayName;

const TreeIcon = ({
  item,
  isOpen,
  isSelected,
  default: defaultIcon,
}: {
  item: TreeDataItem;
  isOpen?: boolean;
  isSelected?: boolean;
  default?: any;
}) => {
  let Icon = defaultIcon;
  if (isSelected && item.selectedIcon) {
    Icon = item.selectedIcon;
  } else if (isOpen && item.openIcon) {
    Icon = item.openIcon;
  } else if (item.icon) {
    Icon = item.icon;
  }
  return Icon ? <Icon className="h-4 w-4 shrink-0 mr-2" /> : <></>;
};

const TreeActions = ({
  children,
  isSelected,
}: {
  children: React.ReactNode;
  isSelected: boolean;
}) => {
  return (
    <div
      className={cn(
        isSelected ? 'block' : 'hidden',
        'absolute right-3 group-hover:block',
      )}
    >
      {children}
    </div>
  );
};

export { TreeView, type TreeDataItem };
