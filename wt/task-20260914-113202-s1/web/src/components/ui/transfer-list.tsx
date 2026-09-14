'use client';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  ChevronLeftIcon,
  ChevronRightIcon,
  SquareCheckIcon,
  SquareIcon,
} from 'lucide-react';
import React, { ReactNode, memo, useCallback, useEffect } from 'react';

export type TransferListItemType = {
  key: string;
  label: string;
  selected?: boolean;
  disabled?: boolean;
};

export enum TransferListMoveDirection {
  Left = 'left',
  Right = 'right',
}

export type TransferListProps = {
  items: TransferListItemType[];
  targetKeys?: string[];
  onChange?(
    targetKeys: string[],
    direction: TransferListMoveDirection,
    moveKeys: string[],
  ): void;
} & {
  children?(item: TransferListItemType): ReactNode;
};

export const TransferList = memo(function ({
  items,
  onChange,
  targetKeys,
  children,
}: TransferListProps) {
  const [leftList, setLeftList] = React.useState<TransferListItemType[]>([]);
  const [rightList, setRightList] = React.useState<TransferListItemType[]>([]);
  const [leftSearch, setLeftSearch] = React.useState('');
  const [rightSearch, setRightSearch] = React.useState('');

  const moveToRight = useCallback(() => {
    const selectedItems = leftList.filter((item) => item.selected);
    const rightItems = [...rightList, ...selectedItems];
    setRightList(rightItems);
    setLeftList(leftList.filter((item) => !item.selected));
    onChange?.(
      rightItems.map((x) => x.key),
      TransferListMoveDirection.Right,
      selectedItems.map((x) => x.key),
    );
  }, [leftList, onChange, rightList]);

  const moveToLeft = useCallback(() => {
    const selectedItems = rightList.filter((item) => item.selected);
    setLeftList((list) => [...list, ...selectedItems]);
    const rightItems = rightList.filter((item) => !item.selected);
    setRightList(rightItems);
    onChange?.(
      rightItems.map((x) => x.key),
      TransferListMoveDirection.Left,
      selectedItems.map((x) => x.key),
    );
  }, [onChange, rightList]);

  const toggleSelection = useCallback(
    (
      list: TransferListItemType[],
      setList: React.Dispatch<React.SetStateAction<TransferListItemType[]>>,
      key: string,
    ) => {
      const updatedList = list.map((item) => {
        if (item.key === key) {
          return { ...item, selected: !item.selected };
        }
        return item;
      });

      setList(updatedList);
    },
    [],
  );

  useEffect(() => {
    const leftItems = items.filter(
      (x) => !targetKeys?.some((y) => y === x.key),
    );
    setLeftList(leftItems);
    const rightItems = items.filter((x) =>
      targetKeys?.some((y) => y === x.key),
    );
    setRightList(rightItems);
  }, [items, targetKeys]);

  return (
    <div className="flex space-x-4">
      <div className="w-1/2 shadow-sm bg-background rounded-sm">
        <div className="flex items-center justify-between">
          <Input
            placeholder="Search"
            className="rounded-br-none rounded-bl-none rounded-tr-none focus-visible:ring-0 focus-visible:border-blue-500"
            value={leftSearch}
            onChange={(e) => setLeftSearch(e.target.value)}
          />
          <Button
            className="rounded-tl-none rounded-bl-none rounded-br-none border-l-0"
            onClick={moveToRight}
            size="icon"
            variant="outline"
          >
            <ChevronRightIcon className="h-4 w-4" />
          </Button>
        </div>
        <ul className="h-[200px] border-l border-r border-b rounded-br-sm rounded-bl-sm p-1.5 overflow-y-scroll">
          {leftList
            .filter((item) =>
              item.label.toLowerCase().includes(leftSearch.toLowerCase()),
            )
            .map((item) => (
              <li
                className="flex items-center gap-1.5 text-sm hover:bg-muted rounded-sm"
                key={item.key}
              >
                <button
                  type={'button'}
                  className="flex items-center gap-1.5 w-full p-1.5"
                  onClick={() =>
                    toggleSelection(leftList, setLeftList, item.key)
                  }
                >
                  {item.selected ? (
                    <SquareCheckIcon className="h-5 w-5 text-muted-foreground/50" />
                  ) : (
                    <SquareIcon className="h-5 w-5 text-muted-foreground/50" />
                  )}
                  {item.label}
                </button>
              </li>
            ))}
        </ul>
      </div>

      <div className="w-1/2 shadow-sm bg-background rounded-sm">
        <div className="flex items-center justify-between">
          <Button
            className="rounded-tr-none rounded-br-none rounded-bl-none border-r-0"
            onClick={moveToLeft}
            size="icon"
            variant="outline"
          >
            <ChevronLeftIcon className="h-4 w-4" />
          </Button>
          <Input
            placeholder="Search"
            className="rounded-bl-none rounded-br-none rounded-tl-none focus-visible:ring-0 focus-visible:border-blue-500"
            value={rightSearch}
            onChange={(e) => setRightSearch(e.target.value)}
          />
        </div>
        <ul className="h-[200px] border-l border-r border-b rounded-br-sm rounded-bl-sm p-1.5 overflow-y-scroll">
          {rightList
            .filter((item) =>
              item.label.toLowerCase().includes(rightSearch.toLowerCase()),
            )
            .map((item) => (
              <li
                className="flex items-center gap-1.5 text-sm hover:bg-muted rounded-sm group"
                key={item.key}
              >
                <button
                  type="button"
                  className="flex items-center gap-1.5 p-1.5"
                  onClick={() =>
                    toggleSelection(rightList, setRightList, item.key)
                  }
                >
                  {item.disabled ? (
                    <span className="size-4"></span>
                  ) : item.selected ? (
                    <SquareCheckIcon className="h-4 w-4 text-muted-foreground/50" />
                  ) : (
                    <SquareIcon className="h-4 w-4 text-muted-foreground/50" />
                  )}
                  {item.label}
                </button>
                {children?.(item)}
              </li>
            ))}
        </ul>
      </div>
    </div>
  );
});

TransferList.displayName = 'TransferList';
