'use client';

import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';
import { LucidePencil } from 'lucide-react';
import { ChangeEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useEditableField } from './use-editable-field';

export interface EditableFieldProps {
  /** Form field name */
  name: string;
  /** Placeholder text when empty */
  placeholder?: string;
  /** Whether field is required */
  required?: boolean;
  /** Whether to show edit icon */
  showEditIcon?: boolean;
  /** Custom className for the container */
  className?: string;
  /** Custom className for the input */
  inputClassName?: string;
  /** Custom className for the display text */
  displayClassName?: string;
  /** Aria label for accessibility */
  ariaLabel?: string;
}

export function EditableField({
  name,
  placeholder,
  required = true,
  showEditIcon = true,
  className,
  inputClassName,
  displayClassName,
  ariaLabel,
}: EditableFieldProps) {
  const { t } = useTranslation();
  const { isEditing, inputRef, handleEnterEdit, handleKeyDown, handleBlur } =
    useEditableField({ required });

  const finalPlaceholder = placeholder ?? t('common.namePlaceholder');

  return (
    <RAGFlowFormItem
      name={name}
      className={cn('flex items-center gap-1.5', className)}
      required={required}
    >
      {(field) =>
        isEditing ? (
          <Input
            ref={inputRef as React.RefObject<HTMLInputElement>}
            name={field.name}
            value={field.value || ''}
            onChange={(e: ChangeEvent<HTMLInputElement>) =>
              field.onChange(e.target.value)
            }
            onBlur={() => {
              field.onBlur();
              handleBlur(field.value || '', field.onChange);
            }}
            onKeyDown={handleKeyDown}
            placeholder={finalPlaceholder}
            className={cn(
              'h-7 text-base font-medium px-2 py-0.5',
              inputClassName,
            )}
            aria-label={ariaLabel}
          />
        ) : (
          <div className="flex items-center gap-1.5">
            <span
              className={cn(
                'text-base font-medium text-text-primary truncate flex-1 min-w-0',
                !field.value && 'text-text-secondary italic',
                displayClassName,
              )}
            >
              {field.value || finalPlaceholder}
            </span>
            {showEditIcon && (
              <button
                type="button"
                onClick={() => handleEnterEdit(field.value || '')}
                className="p-1 text-text-secondary hover:text-text-primary transition-colors focus:outline-none focus:ring-1 focus:ring-accent-primary rounded shrink-0"
                aria-label={ariaLabel ? `Edit ${ariaLabel}` : 'Edit'}
              >
                <LucidePencil className="w-3.5 h-3.5" />
              </button>
            )}
          </div>
        )
      }
    </RAGFlowFormItem>
  );
}

export default EditableField;
