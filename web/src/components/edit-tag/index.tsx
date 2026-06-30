import { PlusOutlined } from '@ant-design/icons';
import React, { useEffect, useRef, useState } from 'react';

import { cn } from '@/lib/utils';
import { Trash2 } from 'lucide-react';
import { Button } from '../ui/button';
import { Input } from '../ui/input';
import { Tooltip, TooltipContent, TooltipTrigger } from '../ui/tooltip';
interface EditTagsProps {
  value?: string[];
  onChange?: (tags: string[]) => void;
  disabled?: boolean;
  addButtonTestId?: string;
  inputTestId?: string;
}

const EditTag = React.forwardRef<HTMLDivElement, EditTagsProps>(
  function EditTag(
    { value = [], onChange, disabled, addButtonTestId, inputTestId },
    ref,
  ) {
    const [inputVisible, setInputVisible] = useState(false);
    const [inputValue, setInputValue] = useState('');
    const inputRef = useRef<HTMLInputElement>(null);

    useEffect(() => {
      if (inputVisible) {
        inputRef.current?.focus();
      }
    }, [inputVisible]);

    const handleClose = (removedTag: string) => {
      const newTags = value?.filter((tag) => tag !== removedTag);
      onChange?.(newTags ?? []);
    };

    const showInput = () => {
      setInputVisible(true);
    };

    const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
      setInputValue(e.target.value);
    };

    const handleInputConfirm = () => {
      if (inputValue && value) {
        const newTags = inputValue
          .split(';')
          .map((tag) => tag.trim())
          .filter((tag) => tag && !value.includes(tag));
        onChange?.([...value, ...newTags]);
      }
      setInputVisible(false);
      setInputValue('');
    };

    const forMap = (tag: string) => {
      return (
        <Tooltip key={tag}>
          <TooltipContent side="top">{tag}</TooltipContent>
          <TooltipTrigger asChild>
            <div
              className={cn(
                'w-fit h-8 flex items-center justify-center gap-2 border border-border-button rounded-sm bg-bg-card',
                disabled ? 'px-2' : 'ps-2 pe-1',
              )}
            >
              <div className="flex gap-2 items-center">
                <div className="max-w-80 whitespace-nowrap overflow-hidden text-ellipsis">
                  {tag}
                </div>
                {!disabled && (
                  <Button
                    variant="delete"
                    size="icon-xs"
                    onClick={(e) => {
                      e.preventDefault();
                      handleClose(tag);
                    }}
                  >
                    <Trash2 className="size-[1em]" />
                  </Button>
                )}
              </div>
            </div>
          </TooltipTrigger>
        </Tooltip>
      );
    };

    const tagChild = value?.map(forMap);

    return (
      <div ref={ref}>
        {inputVisible && (
          <Input
            ref={inputRef}
            type="text"
            className="h-8 bg-bg-card mb-1"
            value={inputValue}
            onChange={handleInputChange}
            onBlur={handleInputConfirm}
            disabled={disabled}
            data-testid={inputTestId}
            onKeyDown={(e) => {
              if (e?.key === 'Enter') {
                handleInputConfirm();
              }
            }}
          />
        )}
        <div className="flex gap-2 py-1 flex-wrap">
          {Array.isArray(tagChild) && tagChild.length > 0 && <>{tagChild}</>}

          {!inputVisible && !disabled && (
            <Button
              variant="outline"
              size="icon"
              onClick={showInput}
              disabled={disabled}
              data-testid={addButtonTestId}
            >
              <PlusOutlined />
            </Button>
          )}
        </div>
      </div>
    );
  },
);

export default EditTag;
