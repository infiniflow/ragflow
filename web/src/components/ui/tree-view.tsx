// https://github.com/MrLightful/shadcn-tree-view

'use client';

import { cn } from '@/lib/utils';
import * as AccordionPrimitive from '@radix-ui/react-accordion';
import { cva } from 'class-variance-authority';
import { ChevronRight } from 'lucide-react';
import React from 'react';

const treeVariants = cva(
  'group hover:before:opacity-100 before:absolute before:rounded-lg before:left-0 px-2 before:w-full before:opacity-0 before:bg-accent/70 before:h-[2rem] before:-z-10 text-text-secondary',
);

const selectedTreeVariants = cva(
  'before:opacity-100 before:bg-bg-card text-accent-foreground',
);

export interface TreeDataItem {
  id: string;
  name: string;
  entityType?: string;
  icon?: any;
  selectedIcon?: any;
  openIcon?: any;
  children?: TreeDataItem[];
  hasChildren?: boolean;
  actions?: React.ReactNode;
  onClick?: () => void;
  /** Fires when the chevron expands a collapsed branch (split-click mode). */
  onExpand?: () => void;
}

type TreeProps = React.HTMLAttributes<HTMLDivElement> & {
  data: TreeDataItem[] | TreeDataItem;
  initialSelectedItemId?: string;
  onSelectChange?: (item: TreeDataItem | undefined) => void;
  expandAll?: boolean;
  /**
   * Defaults to true: clicking anywhere on a branch row toggles expansion.
   * When false, only the chevron toggles expansion (firing `onExpand`) and
   * the rest of the row just selects the item like a leaf (firing `onClick`).
   */
  expandOnRowClick?: boolean;
  defaultNodeIcon?: any;
  defaultLeafIcon?: any;
};

const TreeItemLabel = ({ item }: { item: TreeDataItem }) => {
  if (!item.entityType) {
    return (
      <span className="flex-grow truncate text-sm text-left">{item.name}</span>
    );
  }

  const isTitle = item.entityType === 'title';
  return (
    <span className="flex min-w-0 flex-grow items-center gap-2">
      <span
        className={cn(
          'truncate text-sm',
          isTitle && 'font-medium text-text-primary',
        )}
      >
        {item.name}
      </span>
      <span
        className={cn(
          'shrink-0 rounded px-1.5 py-0.5 text-[10px] leading-none',
          isTitle
            ? 'bg-accent/15 text-accent-foreground'
            : 'bg-bg-card text-text-secondary',
        )}
      >
        {item.entityType}
      </span>
    </span>
  );
};

const TreeView = React.forwardRef<HTMLDivElement, TreeProps>(
  (
    {
      data,
      initialSelectedItemId,
      onSelectChange,
      expandAll,
      expandOnRowClick = true,
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
          expandOnRowClick={expandOnRowClick}
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
      expandOnRowClick = true,
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
              {item.hasChildren ||
              (item.children && item.children.length > 0) ? (
                <TreeNode
                  item={item}
                  selectedItemId={selectedItemId}
                  expandedItemIds={expandedItemIds}
                  handleSelectChange={handleSelectChange}
                  expandOnRowClick={expandOnRowClick}
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
  expandOnRowClick = true,
}: {
  item: TreeDataItem;
  handleSelectChange: (item: TreeDataItem | undefined) => void;
  expandedItemIds: string[];
  selectedItemId?: string;
  defaultNodeIcon?: any;
  defaultLeafIcon?: any;
  expandOnRowClick?: boolean;
}) => {
  const [value, setValue] = React.useState(
    expandedItemIds.includes(item.id) ? [item.id] : [],
  );
  const isOpen = value.includes(item.id);
  const isSelected = selectedItemId === item.id;

  const handleSelect = () => {
    handleSelectChange(item);
    item.onClick?.();
  };

  // Split-click mode: the chevron toggles expansion by itself, so its click
  // must not bubble up to the row (which would also select the node).
  const handleChevronClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!isOpen) {
      item.onExpand?.();
    }
  };

  const content = (
    <AccordionContent className="ml-4 pl-1 border-l">
      <TreeItem
        data={item.children ?? []}
        selectedItemId={selectedItemId}
        handleSelectChange={handleSelectChange}
        expandedItemIds={expandedItemIds}
        expandOnRowClick={expandOnRowClick}
        defaultLeafIcon={defaultLeafIcon}
        defaultNodeIcon={defaultNodeIcon}
      />
    </AccordionContent>
  );

  if (!expandOnRowClick) {
    return (
      <AccordionPrimitive.Root
        type="multiple"
        value={value}
        onValueChange={setValue}
      >
        <AccordionPrimitive.Item value={item.id}>
          <AccordionPrimitive.Header>
            <div
              className={cn(
                'flex flex-1 w-full items-center py-2 cursor-pointer',
                treeVariants(),
                isSelected && selectedTreeVariants(),
              )}
              onClick={handleSelect}
            >
              <AccordionPrimitive.Trigger
                className="mr-1 shrink-0 data-[state=open]:[&>svg]:rotate-90"
                onClick={handleChevronClick}
              >
                <ChevronRight className="h-4 w-4 transition-transform duration-200 text-accent-foreground/50" />
              </AccordionPrimitive.Trigger>
              <TreeIcon
                item={item}
                isSelected={isSelected}
                isOpen={isOpen}
                default={defaultNodeIcon}
              />
              <TreeItemLabel item={item} />
              <TreeActions isSelected={isSelected}>{item.actions}</TreeActions>
            </div>
          </AccordionPrimitive.Header>
          {content}
        </AccordionPrimitive.Item>
      </AccordionPrimitive.Root>
    );
  }

  return (
    <AccordionPrimitive.Root
      type="multiple"
      value={value}
      onValueChange={(s) => setValue(s)}
    >
      <AccordionPrimitive.Item value={item.id}>
        <AccordionTrigger
          className={cn(treeVariants(), isSelected && selectedTreeVariants())}
          onClick={handleSelect}
        >
          <TreeIcon
            item={item}
            isSelected={isSelected}
            isOpen={isOpen}
            default={defaultNodeIcon}
          />
          <TreeItemLabel item={item} />
          <TreeActions isSelected={isSelected}>{item.actions}</TreeActions>
        </AccordionTrigger>
        {content}
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
        <TreeItemLabel item={item} />
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

export { TreeView };
