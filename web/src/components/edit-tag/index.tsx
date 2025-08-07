import { PlusOutlined } from '@ant-design/icons';
import { TweenOneGroup } from 'rc-tween-one';
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

const EditTag = ({ value = [], onChange }: EditTagsProps) => {
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
      <HoverCard>
        <HoverCardContent side="top">{tag}</HoverCardContent>
        <HoverCardTrigger>
          <div
            key={tag}
            className="w-fit flex items-center justify-center gap-2 border-dashed border px-1 rounded-sm bg-bg-card"
          >
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
      {inputVisible ? (
        <Input
          ref={inputRef}
          type="text"
          className="h-8 bg-bg-card"
          value={inputValue}
          onChange={handleInputChange}
          onBlur={handleInputConfirm}
          onKeyDown={(e) => {
            if (e?.key === 'Enter') {
              handleInputConfirm();
            }
          }}
        />
      ) : (
        <Button
          variant="dashed"
          className="w-fit flex items-center justify-center gap-2 bg-bg-card"
          onClick={showInput}
          style={tagPlusStyle}
        >
          <PlusOutlined />
        </Button>
      )}
      {Array.isArray(tagChild) && tagChild.length > 0 && (
        <TweenOneGroup
          className="flex gap-2 flex-wrap mt-2"
          enter={{
            scale: 0.8,
            opacity: 0,
            type: 'from',
            duration: 100,
          }}
          onEnd={(e) => {
            if (e.type === 'appear' || e.type === 'enter') {
              (e.target as any).style = 'display: inline-block';
            }
          }}
          leave={{ opacity: 0, width: 0, scale: 0, duration: 200 }}
          appear={false}
        >
          {tagChild}
        </TweenOneGroup>
      )}
    </div>
  );
};

export default EditTag;
