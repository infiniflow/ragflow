import { PlusOutlined } from '@ant-design/icons';
import React, { useEffect, useRef, useState } from 'react';

import { X } from 'lucide-react';
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
}

const EditTag = React.forwardRef<HTMLDivElement, EditTagsProps>(
  ({ value = [], onChange }: EditTagsProps) => {
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
            <div className="w-fit flex items-center justify-center gap-2 border-dashed border px-2 py-1 rounded-sm bg-bg-card">
              <div className="flex gap-2 items-center">
                <div className="max-w-80 overflow-hidden text-ellipsis">
                  {tag}
                </div>
                <X
                  className="w-4 h-4 text-muted-foreground hover:text-primary"
                  onClick={(e) => {
                    e.preventDefault();
                    handleClose(tag);
                  }}
                />
              </div>
            </div>
          </HoverCardTrigger>
        </HoverCard>
      );
    };

    const tagChild = value?.map(forMap);

    const tagPlusStyle: React.CSSProperties = {
      borderStyle: 'dashed',
    };

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
            onKeyDown={(e) => {
              if (e?.key === 'Enter') {
                handleInputConfirm();
              }
            }}
          />
        )}
        <div className="flex gap-2 py-1">
          {Array.isArray(tagChild) && tagChild.length > 0 && <>{tagChild}</>}
          {!inputVisible && (
            <Button
              variant="dashed"
              className="w-fit flex items-center justify-center gap-2 bg-bg-card"
              onClick={showInput}
              style={tagPlusStyle}
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
