'use client';

import type { ToggleListOption } from '@/components/ui/toggle-list';
import { ToggleList } from '@/components/ui/toggle-list';
import { useTranslate } from '@/hooks/common-hooks';
import { IProviderModelItem } from '@/interfaces/request/llm';
import { Plus } from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import type { AddCustomModelDialogFields } from './add-custom-model-dialog';
import { AddCustomModelDialog } from './add-custom-model-dialog';

export interface AddableToggleListProps {
  /** Trigger button text */
  btnText: React.ReactNode;
  /** ToggleList options */
  options: ToggleListOption<string | null>[];
  /** Show search input */
  searchable?: boolean;
  /** Search placeholder */
  searchPlaceholder?: string;
  /** Empty state text */
  emptyText?: React.ReactNode;
  /** Search loading indicator */
  searchLoading?: boolean;
  /** Max height of the list */
  maxHeight?: number;
  /** ToggleList expand/collapse notification */
  onOpenChange?: (open: boolean) => void;
  /** Called when user submits a new custom model */
  onAdd: (item: IProviderModelItem) => void | Promise<void>;
  /** Dialog title */
  dialogTitle: React.ReactNode;
  /** Fields for the dialog form */
  dialogFields: AddCustomModelDialogFields[];
  /** Submit button text */
  dialogSubmitText?: React.ReactNode;
  /** Cancel button text */
  dialogCancelText?: React.ReactNode;
  /** Container className */
  className?: string;
  /** Trigger button className */
  buttonClassName?: string;
  /** Handle selection of models (for auto-checking new items) */
  handleSelectModel: (model: IProviderModelItem) => void;
  /** Existing model names for uniqueness validation */
  existingNames: string[];
}

/**
 * Wrapper around ToggleList that renders a pinned "Add custom model" footer
 * below the scrollable options. The footer slot is provided by ToggleList,
 * so the action stays visible regardless of how many options are scrolled.
 *
 * Clicking the footer button opens a dialog; submitting the dialog pushes
 * the new model through `onAdd` and then auto-selects it via
 * `handleSelectModel` (which is the sole owner of selection state).
 */
export const AddableToggleList = ({
  btnText,
  options,
  searchable,
  searchPlaceholder,
  emptyText,
  searchLoading,
  maxHeight,
  onOpenChange,
  onAdd,
  dialogTitle,
  dialogFields,
  dialogSubmitText,
  dialogCancelText,
  className,
  buttonClassName,
  handleSelectModel,
  existingNames,
}: AddableToggleListProps) => {
  const { t } = useTranslate('setting');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [dialogLoading, setDialogLoading] = useState(false);

  // Pinned footer rendered below the scrollable items. The footer is part
  // of ToggleList's dropdown panel (outside the scrollable area) so it
  // never scrolls away.
  const footerNode = useMemo(
    () => (
      <div
        role="button"
        tabIndex={0}
        aria-label={t('addCustomModel')}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            setDialogOpen(true);
          }
        }}
        onClick={() => setDialogOpen(true)}
        className="flex items-center justify-center gap-2 px-3 py-4 cursor-pointer bg-bg-card m-2 rounded-md outline-none hover:bg-border-button"
      >
        <Plus
          className="size-4 shrink-0 text-text-secondary"
          aria-hidden="true"
        />
        <span className="text-sm text-text-secondary">
          {t('addCustomModel')}
        </span>
      </div>
    ),
    [t],
  );

  // Handle dialog submission
  const handleDialogSubmit = useCallback(
    async (item: IProviderModelItem) => {
      setDialogLoading(true);
      try {
        const result = onAdd(item);
        if (result instanceof Promise) {
          await result;
        }
        // Auto-select the newly added model. The parent does NOT push the
        // item into its own selection state during `onAdd`; this toggle
        // is the sole writer of selection.
        handleSelectModel(item);
        setDialogOpen(false);
      } catch {
        // Error handling is done in the dialog
      } finally {
        setDialogLoading(false);
      }
    },
    [onAdd, handleSelectModel],
  );

  return (
    <>
      <ToggleList
        className={className}
        btnText={btnText}
        options={options}
        searchable={searchable}
        searchPlaceholder={searchPlaceholder}
        emptyText={emptyText}
        searchLoading={searchLoading}
        onOpenChange={onOpenChange}
        maxHeight={maxHeight}
        buttonClassName={buttonClassName}
        footer={footerNode}
      />
      <AddCustomModelDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        title={dialogTitle}
        fields={dialogFields}
        onSubmit={handleDialogSubmit}
        submitText={dialogSubmitText}
        cancelText={dialogCancelText}
        loading={dialogLoading}
        existingNames={existingNames}
      />
    </>
  );
};
