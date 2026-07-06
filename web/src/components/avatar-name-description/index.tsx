'use client';

import { AvatarUpload } from '@/components/avatar-upload';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { cn } from '@/lib/utils';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { EditableField } from './editable-field';
import { EditableTextarea } from './editable-textarea';

export interface AvatarNameDescriptionProps {
  /** Form field name for avatar/icon (default: 'icon') */
  avatarField?: string;
  /** Form field name for name/title (default: 'name') */
  nameField?: string;
  /** Form field name for description (default: 'description') */
  descriptionField?: string;
  /** Label for name field */
  nameLabel?: string;
  /** Label for description field */
  descriptionLabel?: string;
  /** Placeholder for name input */
  namePlaceholder?: string;
  /** Placeholder for description input */
  descriptionPlaceholder?: string;
  /** Custom className for the container */
  className?: string;
  /** Whether name is required */
  nameRequired?: boolean;
  /** Whether to show edit icons */
  showEditIcons?: boolean;
}

export function AvatarNameDescription({
  avatarField = 'icon',
  nameField = 'name',
  descriptionField = 'description',
  nameLabel,
  descriptionLabel,
  namePlaceholder,
  descriptionPlaceholder,
  className,
  nameRequired = true,
  showEditIcons = true,
}: AvatarNameDescriptionProps) {
  const { t } = useTranslation();
  useFormContext(); // Ensure component is used within FormProvider

  return (
    <div className={cn('flex gap-3', className)}>
      <RAGFlowFormItem name={avatarField}>
        <AvatarUpload tips={''} />
      </RAGFlowFormItem>

      {/* Name & Description Section */}
      <div className="flex-1 min-w-0 pt-1 space-y-1">
        {/* Name Row - Using EditableField component */}
        <EditableField
          name={nameField}
          placeholder={namePlaceholder}
          required={nameRequired}
          showEditIcon={showEditIcons}
          ariaLabel={nameLabel ?? t('common.name')}
        />

        {/* Description Row - Using EditableTextarea component */}
        <div className="mt-0.5">
          <EditableTextarea
            name={descriptionField}
            placeholder={descriptionPlaceholder}
            showEditIcon={showEditIcons}
            ariaLabel={descriptionLabel ?? t('common.description')}
          />
        </div>
      </div>
    </div>
  );
}

export default AvatarNameDescription;
