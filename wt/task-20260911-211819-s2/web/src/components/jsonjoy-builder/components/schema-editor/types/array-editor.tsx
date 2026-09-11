import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { useId, useMemo, useState } from 'react';
import { useTranslation } from '../../../hooks/use-translation';
import { getArrayItemsSchema } from '../../../lib/schema-editor';
import { cn } from '../../../lib/utils';
import type { ObjectJSONSchema, SchemaType } from '../../../types/json-schema';
import { isBooleanSchema, withObjectSchema } from '../../../types/json-schema';
import TypeDropdown from '../type-dropdown';
import type { TypeEditorProps } from '../type-editor';
import TypeEditor from '../type-editor';

const ArrayEditor: React.FC<TypeEditorProps> = ({
  schema,
  validationNode,
  onChange,
  depth = 0,
}) => {
  const t = useTranslation();
  const [minItems, setMinItems] = useState<number | undefined>(
    withObjectSchema(schema, (s) => s.minItems, undefined),
  );
  const [maxItems, setMaxItems] = useState<number | undefined>(
    withObjectSchema(schema, (s) => s.maxItems, undefined),
  );
  const [uniqueItems, setUniqueItems] = useState<boolean>(
    withObjectSchema(schema, (s) => s.uniqueItems || false, false),
  );

  const minItemsId = useId();
  const maxItemsId = useId();
  const uniqueItemsId = useId();

  // Get the array's item schema
  const itemsSchema = getArrayItemsSchema(schema) || { type: 'string' };

  // Get the type of the array items
  const itemType = withObjectSchema(
    itemsSchema,
    (s) => (s.type || 'string') as SchemaType,
    'string' as SchemaType,
  );

  // Handle validation settings change
  const handleValidationChange = () => {
    const validationProps: ObjectJSONSchema = {
      type: 'array',
      ...(isBooleanSchema(schema) ? {} : schema),
      minItems: minItems,
      maxItems: maxItems,
      uniqueItems: uniqueItems || undefined,
    };

    // Keep the items schema
    if (validationProps.items === undefined && itemsSchema) {
      validationProps.items = itemsSchema;
    }

    // Clean up undefined values
    const propsToKeep: Record<string, unknown> = {};
    for (const [key, value] of Object.entries(validationProps)) {
      if (value !== undefined) {
        propsToKeep[key] = value;
      }
    }

    onChange(propsToKeep as ObjectJSONSchema);
  };

  // Handle item schema changes
  const handleItemSchemaChange = (updatedItemSchema: ObjectJSONSchema) => {
    const updatedSchema: ObjectJSONSchema = {
      type: 'array',
      ...(isBooleanSchema(schema) ? {} : schema),
      items: updatedItemSchema,
    };

    onChange(updatedSchema);
  };

  const minMaxError = useMemo(
    () =>
      validationNode?.validation.errors?.find((err) => err.path[0] === 'minmax')
        ?.message,
    [validationNode],
  );

  const minItemsError = useMemo(
    () =>
      validationNode?.validation.errors?.find(
        (err) => err.path[0] === 'minItems',
      )?.message,
    [validationNode],
  );

  const maxItemsError = useMemo(
    () =>
      validationNode?.validation.errors?.find(
        (err) => err.path[0] === 'maxItems',
      )?.message,
    [validationNode],
  );

  return (
    <div className="space-y-6">
      {/* Array validation settings */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label
            htmlFor={minItemsId}
            className={(!!minMaxError || !!minItemsError) && 'text-destructive'}
          >
            {t.arrayMinimumLabel}
          </Label>
          <Input
            id={minItemsId}
            type="number"
            min={0}
            value={minItems ?? ''}
            onChange={(e) => {
              const value = e.target.value ? Number(e.target.value) : undefined;
              setMinItems(value);
              // Don't update immediately to avoid too many rerenders
            }}
            onBlur={handleValidationChange}
            placeholder={t.arrayMinimumPlaceholder}
            className={cn('h-8', !!minMaxError && 'border-destructive')}
          />
        </div>

        <div className="space-y-2">
          <Label
            htmlFor={maxItemsId}
            className={(!!minMaxError || !!maxItemsError) && 'text-destructive'}
          >
            {t.arrayMaximumLabel}
          </Label>
          <Input
            id={maxItemsId}
            type="number"
            min={0}
            value={maxItems ?? ''}
            onChange={(e) => {
              const value = e.target.value ? Number(e.target.value) : undefined;
              setMaxItems(value);
              // Don't update immediately to avoid too many rerenders
            }}
            onBlur={handleValidationChange}
            placeholder={t.arrayMaximumPlaceholder}
            className={cn('h-8', !!minMaxError && 'border-destructive')}
          />
        </div>
        {(!!minMaxError || !!minItemsError || !!maxItemsError) && (
          <div className="text-xs text-destructive italic md:col-span-2 whitespace-pre-line">
            {[minMaxError, minItemsError ?? maxItemsError]
              .filter(Boolean)
              .join('\n')}
          </div>
        )}
      </div>

      <div className="flex items-center space-x-2">
        <Switch
          id={uniqueItemsId}
          checked={uniqueItems}
          onCheckedChange={(checked) => {
            setUniqueItems(checked);
            setTimeout(handleValidationChange, 0);
          }}
        />
        <Label htmlFor={uniqueItemsId} className="cursor-pointer">
          {t.arrayForceUniqueItemsLabel}
        </Label>
      </div>

      {/* Array item type editor */}
      <div className="space-y-2 pt-4 border-t border-border/40">
        <div className="flex items-center justify-between mb-4">
          <Label>{t.arrayItemTypeLabel}</Label>
          <TypeDropdown
            value={itemType}
            onChange={(newType) => {
              handleItemSchemaChange({
                ...withObjectSchema(itemsSchema, (s) => s, {}),
                type: newType,
              });
            }}
          />
        </div>

        {/* Item schema editor */}
        <TypeEditor
          schema={itemsSchema}
          validationNode={validationNode}
          onChange={handleItemSchemaChange}
          depth={depth + 1}
        />
      </div>
    </div>
  );
};

export default ArrayEditor;
