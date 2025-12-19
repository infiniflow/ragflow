import { PlusOutlined } from '@ant-design/icons';
import React, { useEffect, useRef, useState } from 'react';

import { Trash2 } from 'lucide-react';
import { Button } from '../ui/button';
import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from '../ui/hover-card';
import { Input } from '../ui/input';
interface EditTagsProps {
  value?: string[];
  onChange?: (tags: string[]) => void;
  disabled?: boolean;
}

const EditTag = React.forwardRef<HTMLDivElement, EditTagsProps>(
  ({ value = [], onChange, disabled }: EditTagsProps) => {
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
        <HoverCard key={tag}>
          <HoverCardContent side="top">{tag}</HoverCardContent>
          <HoverCardTrigger asChild>
            <div className="w-fit flex items-center justify-center gap-2 border border-border-button px-2 py-1 rounded-sm bg-bg-card">
              <div className="flex gap-2 items-center">
                <div className="max-w-80 overflow-hidden text-ellipsis">
                  {tag}
                </div>
                {!disabled && (
                  <Trash2
                    size={14}
                    className="text-text-secondary hover:text-state-error"
                    onClick={(e) => {
                      e.preventDefault();
                      handleClose(tag);
                    }}
                  />
                )}
              </div>
            </div>
          </HoverCardTrigger>
        </HoverCard>
      );
    };

    const tagChild = value?.map(forMap);

    return (
      <div>
        {inputVisible && (
          <Input
            ref={inputRef}
            type="text"
            className="h-8 bg-bg-card mb-1"
            value={inputValue}
            onChange={handleInputChange}
            onBlur={handleInputConfirm}
            disabled={disabled}
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
              variant="ghost"
              className="w-fit flex items-center justify-center gap-2 bg-bg-card border-border-button border"
              onClick={showInput}
              disabled={disabled}
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
