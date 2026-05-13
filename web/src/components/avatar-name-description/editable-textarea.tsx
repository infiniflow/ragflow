'use client';

import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Textarea } from '@/components/ui/textarea';
import { cn } from '@/lib/utils';
import { LucidePencil } from 'lucide-react';
import { ChangeEvent, useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

export interface EditableTextareaProps {
  /** Form field name */
  name: string;
  /** Placeholder text when empty */
  placeholder?: string;
  /** Whether to show edit icon */
  showEditIcon?: boolean;
  /** Custom className for the container */
  className?: string;
  /** Custom className for the textarea */
  textareaClassName?: string;
  /** Custom className for the display text */
  displayClassName?: string;
  /** Aria label for accessibility */
  ariaLabel?: string;
  /** Minimum number of rows for textarea */
  minRows?: number;
  /** Maximum number of rows for textarea */
  maxRows?: number;
}

export function EditableTextarea({
  name,
  placeholder,
  showEditIcon = true,
  className,
  textareaClassName,
  displayClassName,
  ariaLabel,
  minRows = 2,
  maxRows = 3,
}: EditableTextareaProps) {
  const { t } = useTranslation();
  const [isEditing, setIsEditing] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const finalPlaceholder = placeholder ?? t('common.descriptionPlaceholder');

  // Auto-focus when entering edit mode and move cursor to end
  useEffect(() => {
    if (isEditing) {
      const frameId = requestAnimationFrame(() => {
        const textarea = textareaRef.current;
        if (textarea) {
          textarea.focus();
          const length = textarea.value.length;
          textarea.setSelectionRange(length, length);
        }
      });
      return () => cancelAnimationFrame(frameId);
    }
  }, [isEditing]);

  const handleEnterEdit = useCallback(() => {
    setIsEditing(true);
  }, []);

  const handleExitEdit = useCallback(() => {
    setIsEditing(false);
  }, []);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === 'Escape') {
        setIsEditing(false);
      }
    },
    [],
  );

  return (
    <div className={cn('flex items-center gap-1.5 group', className)}>
      <RAGFlowFormItem
        name={name}
        className={cn(isEditing ? 'flex-1 w-full' : 'w-auto min-w-0')}
      >
        {(field) =>
          isEditing ? (
            <Textarea
              ref={textareaRef}
              name={field.name}
              value={field.value || ''}
              onChange={(e: ChangeEvent<HTMLTextAreaElement>) =>
                field.onChange(e.target.value)
              }
              onBlur={() => {
                field.onBlur();
                handleExitEdit();
              }}
              onKeyDown={handleKeyDown}
              placeholder={finalPlaceholder}
              className={cn(
                'min-h-[28px] text-sm text-text-secondary resize-none px-2 py-0.5',
                textareaClassName,
              )}
              autoSize={{ minRows, maxRows }}
              aria-label={ariaLabel}
            />
          ) : (
            <p
              className={cn(
                'block w-full text-sm text-text-secondary line-clamp-2 text-left',
                !field.value && 'italic',
                displayClassName,
              )}
              onClick={handleEnterEdit}
              role="button"
              tabIndex={0}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  handleEnterEdit();
                }
              }}
            >
              {field.value || finalPlaceholder}
            </p>
          )
        }
      </RAGFlowFormItem>
      {!isEditing && showEditIcon && (
        <button
          type="button"
          onClick={handleEnterEdit}
          className="p-1 text-text-secondary hover:text-text-primary transition-colors focus:outline-none focus:ring-1 focus:ring-accent-primary rounded shrink-0 opacity-0 group-hover:opacity-100"
          aria-label={ariaLabel ? `Edit ${ariaLabel}` : 'Edit'}
        >
          <LucidePencil className="w-3.5 h-3.5" />
        </button>
      )}
    </div>
  );
}

export default EditableTextarea;
