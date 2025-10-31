import type { FC } from 'react';
import { useTranslation } from '../../hooks/use-translation';
import type { Translation } from '../../i18n/translation-keys';
import { cn } from '../../lib/utils';
import type { SchemaType } from '../../types/json-schema';

interface SchemaTypeSelectorProps {
  id?: string;
  value: SchemaType;
  onChange: (value: SchemaType) => void;
}

interface TypeOption {
  id: SchemaType;
  label: keyof Translation;
  description: keyof Translation;
}

const typeOptions: TypeOption[] = [
  {
    id: 'string',
    label: 'fieldTypeTextLabel',
    description: 'fieldTypeTextDescription',
  },
  {
    id: 'number',
    label: 'fieldTypeNumberLabel',
    description: 'fieldTypeNumberDescription',
  },
  {
    id: 'boolean',
    label: 'fieldTypeBooleanLabel',
    description: 'fieldTypeBooleanDescription',
  },
  {
    id: 'object',
    label: 'fieldTypeObjectLabel',
    description: 'fieldTypeObjectDescription',
  },
  {
    id: 'array',
    label: 'fieldTypeArrayLabel',
    description: 'fieldTypeArrayDescription',
  },
];

const SchemaTypeSelector: FC<SchemaTypeSelectorProps> = ({
  id,
  value,
  onChange,
}) => {
  const t = useTranslation();
  return (
    <div
      id={id}
      className="grid grid-cols-1 xs:grid-cols-2 md:grid-cols-3 gap-2"
    >
      {typeOptions.map((type) => (
        <button
          type="button"
          key={type.id}
          title={t[type.description]}
          className={cn(
            'p-2.5 rounded-lg border-2 text-left transition-all duration-200',
            value === type.id
              ? 'border-primary bg-primary/5 shadow-xs'
              : 'border-border hover:border-primary/30 hover:bg-secondary',
          )}
          onClick={() => onChange(type.id)}
        >
          <div className="font-medium text-sm">{t[type.label]}</div>
          <div className="text-xs text-muted-foreground line-clamp-1">
            {t[type.description]}
          </div>
        </button>
      ))}
    </div>
  );
};

export default SchemaTypeSelector;
