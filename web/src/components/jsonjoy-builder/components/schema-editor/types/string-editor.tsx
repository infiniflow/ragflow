import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { X } from 'lucide-react';
import { useId, useMemo, useState } from 'react';
import { useTranslation } from '../../../hooks/use-translation';
import { cn } from '../../../lib/utils';
import type { ObjectJSONSchema } from '../../../types/json-schema';
import { isBooleanSchema, withObjectSchema } from '../../../types/json-schema';
import type { TypeEditorProps } from '../type-editor';

type Property = 'enum' | 'minLength' | 'maxLength' | 'pattern' | 'format';

const StringEditor: React.FC<TypeEditorProps> = ({
  schema,
  validationNode,
  onChange,
}) => {
  const t = useTranslation();
  const [enumValue, setEnumValue] = useState('');

  const minLengthId = useId();
  const maxLengthId = useId();
  const patternId = useId();
  const formatId = useId();

  // Extract string-specific validations
  const minLength = withObjectSchema(schema, (s) => s.minLength, undefined);
  const maxLength = withObjectSchema(schema, (s) => s.maxLength, undefined);
  const pattern = withObjectSchema(schema, (s) => s.pattern, undefined);
  const format = withObjectSchema(schema, (s) => s.format, undefined);
  const enumValues = withObjectSchema(
    schema,
    (s) => (s.enum as string[]) || [],
    [],
  );

  // Handle validation change
  const handleValidationChange = (property: Property, value: unknown) => {
    // Create a safe base schema
    const baseSchema = isBooleanSchema(schema)
      ? { type: 'string' as const }
      : { ...schema };

    // Get all validation props except type and description
    const { type: _, description: __, ...validationProps } = baseSchema;

    // Create the updated validation schema
    const updatedValidation: ObjectJSONSchema = {
      ...validationProps,
      type: 'string',
      [property]: value,
    };

    // Call onChange with the updated schema (even if there are validation errors)
    onChange(updatedValidation);
  };

  // Handle adding enum value
  const handleAddEnumValue = () => {
    if (!enumValue.trim()) return;

    if (!enumValues.includes(enumValue)) {
      handleValidationChange('enum', [...enumValues, enumValue]);
    }

    setEnumValue('');
  };

  // Handle removing enum value
  const handleRemoveEnumValue = (index: number) => {
    const newEnumValues = [...enumValues];
    newEnumValues.splice(index, 1);

    if (newEnumValues.length === 0) {
      // If empty, remove the enum property entirely
      const baseSchema = isBooleanSchema(schema)
        ? { type: 'string' as const }
        : { ...schema };

      // Use a type safe approach
      if (!isBooleanSchema(baseSchema) && 'enum' in baseSchema) {
        const { enum: _, ...rest } = baseSchema;
        onChange(rest as ObjectJSONSchema);
      } else {
        onChange(baseSchema as ObjectJSONSchema);
      }
    } else {
      handleValidationChange('enum', newEnumValues);
    }
  };

  const minMaxError = useMemo(
    () =>
      validationNode?.validation.errors?.find((err) => err.path[0] === 'length')
        ?.message,
    [validationNode],
  );

  const minLengthError = useMemo(
    () =>
      validationNode?.validation.errors?.find(
        (err) => err.path[0] === 'minLength',
      )?.message,
    [validationNode],
  );

  const maxLengthError = useMemo(
    () =>
      validationNode?.validation.errors?.find(
        (err) => err.path[0] === 'maxLength',
      )?.message,
    [validationNode],
  );

  const patternError = useMemo(
    () =>
      validationNode?.validation.errors?.find(
        (err) => err.path[0] === 'pattern',
      )?.message,
    [validationNode],
  );

  const formatError = useMemo(
    () =>
      validationNode?.validation.errors?.find((err) => err.path[0] === 'format')
        ?.message,
    [validationNode],
  );

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 items-start">
        <div className="space-y-2">
          <Label
            htmlFor={minLengthId}
            className={
              (!!minMaxError || !!minLengthError) && 'text-destructive'
            }
          >
            {t.stringMinimumLengthLabel}
          </Label>
          <Input
            id={minLengthId}
            type="number"
            min={0}
            value={minLength ?? ''}
            onChange={(e) => {
              const value = e.target.value ? Number(e.target.value) : undefined;
              handleValidationChange('minLength', value);
            }}
            placeholder={t.stringMinimumLengthPlaceholder}
            className={cn(
              'h-8',
              (!!minMaxError || !!minLengthError) && 'border-destructive',
            )}
          />
        </div>

        <div className="space-y-2">
          <Label
            htmlFor={maxLengthId}
            className={
              (!!minMaxError || !!maxLengthError) && 'text-destructive'
            }
          >
            {t.stringMaximumLengthLabel}
          </Label>
          <Input
            id={maxLengthId}
            type="number"
            min={0}
            value={maxLength ?? ''}
            onChange={(e) => {
              const value = e.target.value ? Number(e.target.value) : undefined;
              handleValidationChange('maxLength', value);
            }}
            placeholder={t.stringMaximumLengthPlaceholder}
            className={cn(
              'h-8',
              (!!minMaxError || !!maxLengthError) && 'border-destructive',
            )}
          />
        </div>
        {(!!minMaxError || !!minLengthError || !!maxLengthError) && (
          <div className="text-xs text-destructive italic md:col-span-2 whitespace-pre-line">
            {[minMaxError, minLengthError ?? maxLengthError]
              .filter(Boolean)
              .join('\n')}
          </div>
        )}
      </div>

      <div className="space-y-2">
        <Label
          htmlFor={patternId}
          className={!!patternError && 'text-destructive'}
        >
          {t.stringPatternLabel}
        </Label>
        <Input
          id={patternId}
          type="text"
          value={pattern ?? ''}
          onChange={(e) => {
            const value = e.target.value || undefined;
            handleValidationChange('pattern', value);
          }}
          placeholder={t.stringPatternPlaceholder}
          className="h-8"
        />
      </div>

      <div className="space-y-2">
        <Label
          htmlFor={formatId}
          className={!!formatError && 'text-destructive'}
        >
          {t.stringFormatLabel}
        </Label>
        <Select
          value={format || 'none'}
          onValueChange={(value) => {
            handleValidationChange(
              'format',
              value === 'none' ? undefined : value,
            );
          }}
        >
          <SelectTrigger id={formatId} className="h-8">
            <SelectValue placeholder="Select format" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="none">{t.stringFormatNone}</SelectItem>
            <SelectItem value="date-time">{t.stringFormatDateTime}</SelectItem>
            <SelectItem value="date">{t.stringFormatDate}</SelectItem>
            <SelectItem value="time">{t.stringFormatTime}</SelectItem>
            <SelectItem value="email">{t.stringFormatEmail}</SelectItem>
            <SelectItem value="uri">{t.stringFormatUri}</SelectItem>
            <SelectItem value="uuid">{t.stringFormatUuid}</SelectItem>
            <SelectItem value="hostname">{t.stringFormatHostname}</SelectItem>
            <SelectItem value="ipv4">{t.stringFormatIpv4}</SelectItem>
            <SelectItem value="ipv6">{t.stringFormatIpv6}</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div className="space-y-2 pt-2 border-t border-border/40">
        <Label>{t.stringAllowedValuesEnumLabel}</Label>

        <div className="flex flex-wrap gap-2 mb-4">
          {enumValues.length > 0 ? (
            enumValues.map((value) => (
              <div
                key={`enum-string-${value}`}
                className="flex items-center bg-muted/40 border rounded-md px-2 py-1 text-xs"
              >
                <span className="mr-1">{value}</span>
                <button
                  type="button"
                  onClick={() =>
                    handleRemoveEnumValue(enumValues.indexOf(value))
                  }
                  className="text-muted-foreground hover:text-destructive"
                >
                  <X size={12} />
                </button>
              </div>
            ))
          ) : (
            <p className="text-xs text-muted-foreground italic">
              {t.stringAllowedValuesEnumNone}
            </p>
          )}
        </div>

        <div className="flex items-center gap-2">
          <Input
            type="text"
            value={enumValue}
            onChange={(e) => setEnumValue(e.target.value)}
            placeholder={t.stringAllowedValuesEnumAddPlaceholder}
            className="h-8 text-xs flex-1"
            onKeyDown={(e) => e.key === 'Enter' && handleAddEnumValue()}
          />
          <button
            type="button"
            onClick={handleAddEnumValue}
            className="px-3 py-1 h-8 rounded-md bg-secondary text-xs font-medium hover:bg-secondary/80"
          >
            Add
          </button>
        </div>
      </div>
    </div>
  );
};

export default StringEditor;
