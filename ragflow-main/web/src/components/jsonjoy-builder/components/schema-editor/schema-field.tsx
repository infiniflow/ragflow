import React, { Suspense } from 'react';
import { useTranslation } from '../../hooks/use-translation';
import type {
  JSONSchema as JSONSchemaType,
  NewField,
  ObjectJSONSchema,
  SchemaType,
} from '../../types/json-schema';
import {
  asObjectSchema,
  getSchemaDescription,
  withObjectSchema,
} from '../../types/json-schema';
import SchemaPropertyEditor from './schema-property-editor';

// This component is now just a simple wrapper around SchemaPropertyEditor
// to maintain backward compatibility during migration
interface SchemaFieldProps {
  name: string;
  schema: JSONSchemaType;
  required?: boolean;
  onDelete: () => void;
  onEdit: (updatedField: NewField) => void;
  onAddField?: (newField: NewField) => void;
  isNested?: boolean;
  depth?: number;
}

const SchemaField: React.FC<SchemaFieldProps> = (props) => {
  const { name, schema, required = false, onDelete, onEdit, depth = 0 } = props;

  // Handle name change
  const handleNameChange = (newName: string) => {
    if (newName === name) return;

    // Get type in a safe way
    const type = withObjectSchema(
      schema,
      (s) => s.type || 'object',
      'object',
    ) as SchemaType;

    // Get description in a safe way
    const description = getSchemaDescription(schema);

    onEdit({
      name: newName,
      type: Array.isArray(type) ? type[0] : type,
      description,
      required,
      validation: asObjectSchema(schema),
    });
  };

  // Handle required status change
  const handleRequiredChange = (isRequired: boolean) => {
    if (isRequired === required) return;

    // Get type in a safe way
    const type = withObjectSchema(
      schema,
      (s) => s.type || 'object',
      'object',
    ) as SchemaType;

    // Get description in a safe way
    const description = getSchemaDescription(schema);

    onEdit({
      name,
      type: Array.isArray(type) ? type[0] : type,
      description,
      required: isRequired,
      validation: asObjectSchema(schema),
    });
  };

  // Handle schema change
  const handleSchemaChange = (newSchema: ObjectJSONSchema) => {
    // Type will be defined in the schema
    const type = newSchema.type || 'object';

    // Description will be defined in the schema
    const description = newSchema.description || '';

    onEdit({
      name,
      type: Array.isArray(type) ? type[0] : type,
      description,
      required,
      validation: newSchema,
    });
  };

  return (
    <SchemaPropertyEditor
      name={name}
      schema={schema}
      required={required}
      onDelete={onDelete}
      onNameChange={handleNameChange}
      onRequiredChange={handleRequiredChange}
      onSchemaChange={handleSchemaChange}
      depth={depth}
    />
  );
};

export default SchemaField;

// ExpandButton - extract for reuse
export interface ExpandButtonProps {
  expanded: boolean;
  onClick: () => void;
}

export const ExpandButton: React.FC<ExpandButtonProps> = ({
  expanded,
  onClick,
}) => {
  const t = useTranslation();
  const ChevronDown = React.lazy(() =>
    import('lucide-react').then((mod) => ({ default: mod.ChevronDown })),
  );
  const ChevronRight = React.lazy(() =>
    import('lucide-react').then((mod) => ({ default: mod.ChevronRight })),
  );

  return (
    <button
      type="button"
      className="text-muted-foreground hover:text-foreground transition-colors"
      onClick={onClick}
      aria-label={expanded ? t.collapse : t.expand}
    >
      <Suspense fallback={<div className="w-[18px] h-[18px]" />}>
        {expanded ? <ChevronDown size={18} /> : <ChevronRight size={18} />}
      </Suspense>
    </button>
  );
};

// FieldActions - extract for reuse
export interface FieldActionsProps {
  onDelete: () => void;
}

export const FieldActions: React.FC<FieldActionsProps> = ({ onDelete }) => {
  const t = useTranslation();
  const X = React.lazy(() =>
    import('lucide-react').then((mod) => ({ default: mod.X })),
  );

  return (
    <div className="flex items-center gap-1 text-muted-foreground">
      <button
        type="button"
        onClick={onDelete}
        className="p-1 rounded-md hover:bg-secondary hover:text-destructive transition-colors opacity-0 group-hover:opacity-100"
        aria-label={t.fieldDelete}
      >
        <Suspense fallback={<div className="w-[16px] h-[16px]" />}>
          <X size={16} />
        </Suspense>
      </button>
    </div>
  );
};
