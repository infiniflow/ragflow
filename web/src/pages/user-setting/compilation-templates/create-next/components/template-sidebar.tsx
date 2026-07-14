import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { Plus, Trash2 } from 'lucide-react';
import { useCallback, type KeyboardEvent, type MouseEvent } from 'react';
import {
  FieldArrayWithId,
  UseFieldArrayReturn,
  UseFormReturn,
} from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { formatKindLabel } from '@/utils/compilation-template-util';

import { FormSchemaType } from '@/pages/user-setting/compilation-templates/create-next/schema';
import { DefaultTemplateValues } from '@/pages/user-setting/compilation-templates/create-next/utils';

import { useTemplateAddButton } from '../hooks/use-template-add-button';

type TemplateSidebarProps = {
  form: UseFormReturn<FormSchemaType>;
  fields: FieldArrayWithId<FormSchemaType, 'templates'>[];
  append: UseFieldArrayReturn<FormSchemaType, 'templates'>['append'];
  remove: UseFieldArrayReturn<FormSchemaType, 'templates'>['remove'];
  kindOptions: { label: string; value: string }[];
  selectedTemplateIndex: number;
  onSelectTemplate: (index: number) => void;
};

type TemplateSidebarItemProps = {
  field: FieldArrayWithId<FormSchemaType, 'templates'>;
  index: number;
  template: FormSchemaType['templates'][number] | undefined;
  isActive: boolean;
  canRemove: boolean;
  onSelectTemplate: (index: number) => void;
  onRemoveTemplate: (index: number) => void;
};

function TemplateSidebarItem({
  field,
  index,
  template,
  isActive,
  canRemove,
  onSelectTemplate,
  onRemoveTemplate,
}: TemplateSidebarItemProps) {
  const { t } = useTranslation();

  const handleClick = useCallback(() => {
    onSelectTemplate(index);
  }, [index, onSelectTemplate]);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLDivElement>) => {
      if (e.key === 'Enter' || e.key === ' ') {
        onSelectTemplate(index);
      }
    },
    [index, onSelectTemplate],
  );

  const handleRemoveClick = useCallback(
    (e: MouseEvent<HTMLButtonElement>) => {
      e.stopPropagation();
      onRemoveTemplate(index);
    },
    [index, onRemoveTemplate],
  );

  return (
    <div
      key={field.id}
      role="button"
      tabIndex={0}
      onClick={handleClick}
      onKeyDown={handleKeyDown}
      className={cn(
        'group flex items-center justify-between px-3 py-2 rounded-md text-sm cursor-pointer',
        isActive
          ? 'bg-bg-card text-text-primary'
          : 'text-text-secondary hover:bg-bg-card hover:text-text-primary',
      )}
    >
      <div className="flex items-center min-w-0 flex-1">
        <span className="truncate">
          {template?.name || `${t('setting.template')} #${index + 1}`}
        </span>
        {template?.kind && (
          <span className="ml-2 shrink-0 text-text-secondary">
            {formatKindLabel(template.kind)}
          </span>
        )}
      </div>

      {canRemove && (
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          onClick={handleRemoveClick}
          className="opacity-0 group-hover:opacity-100 focus:opacity-100 text-text-secondary hover:text-state-error"
        >
          <Trash2 className="size-4" />
        </Button>
      )}
    </div>
  );
}

export function TemplateSidebar({
  form,
  fields,
  append,
  remove,
  kindOptions,
  selectedTemplateIndex,
  onSelectTemplate,
}: TemplateSidebarProps) {
  const { t } = useTranslation();
  const { templates, isAddButtonHidden } = useTemplateAddButton(
    form,
    kindOptions,
  );

  const handleAddTemplate = useCallback(() => {
    const firstTemplateLlmId = form.getValues('templates.0.llm_id');
    const nextIndex = fields.length;
    append({
      ...DefaultTemplateValues,
      name: `${t('setting.template')} #${nextIndex + 1}`,
      llm_id: firstTemplateLlmId || '',
    });
    onSelectTemplate(nextIndex);
  }, [append, fields.length, form, onSelectTemplate, t]);

  const handleRemoveTemplate = useCallback(
    (index: number) => {
      remove(index);
      if (selectedTemplateIndex === index) {
        onSelectTemplate(Math.max(0, index - 1));
      } else if (selectedTemplateIndex > index) {
        onSelectTemplate(selectedTemplateIndex - 1);
      }
    },
    [onSelectTemplate, remove, selectedTemplateIndex],
  );

  return (
    <aside className="h-full flex flex-col">
      <div className="flex items-center justify-between px-4 py-3 border-b border-border-button">
        <span className="text-sm font-medium text-text-primary">
          {t('setting.templates')}
        </span>
        {false && !isAddButtonHidden && (
          <Button
            type="button"
            variant="ghost"
            size="icon-xs"
            onClick={handleAddTemplate}
            className="text-text-secondary hover:text-text-primary"
          >
            <Plus className="size-4" />
          </Button>
        )}
      </div>

      <div className="flex-1 overflow-y-auto p-2 space-y-1">
        {fields.map((field, index) => (
          <TemplateSidebarItem
            key={field.id}
            field={field}
            index={index}
            template={templates[index]}
            isActive={selectedTemplateIndex === index}
            canRemove={fields.length > 1}
            onSelectTemplate={onSelectTemplate}
            onRemoveTemplate={handleRemoveTemplate}
          />
        ))}
      </div>
    </aside>
  );
}
