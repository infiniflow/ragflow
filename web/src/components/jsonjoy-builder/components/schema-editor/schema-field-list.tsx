import { useMemo, type FC } from 'react';
import { useTranslation } from '../../hooks/use-translation';
import { getSchemaProperties } from '../../lib/schema-editor';
import type {
  JSONSchema as JSONSchemaType,
  NewField,
  ObjectJSONSchema,
  SchemaType,
} from '../../types/json-schema';
import { buildValidationTree } from '../../types/validation';
import SchemaPropertyEditor from './schema-property-editor';

interface SchemaFieldListProps {
  schema: JSONSchemaType;
  onAddField: (newField: NewField) => void;
  onEditField: (name: string, updatedField: NewField) => void;
  onDeleteField: (name: string) => void;
}

const SchemaFieldList: FC<SchemaFieldListProps> = ({
  schema,
  onEditField,
  onDeleteField,
}) => {
  const t = useTranslation();

  // Get the properties from the schema
  const properties = getSchemaProperties(schema);

  // Get schema type as a valid SchemaType
  const getValidSchemaType = (propSchema: JSONSchemaType): SchemaType => {
    if (typeof propSchema === 'boolean') return 'object';

    // Handle array of types by picking the first one
    const type = propSchema.type;
    if (Array.isArray(type)) {
      return type[0] || 'object';
    }

    return type || 'object';
  };

  // Handle field name change (generates an edit event)
  const handleNameChange = (oldName: string, newName: string) => {
    const property = properties.find((prop) => prop.name === oldName);
    if (!property) return;

    onEditField(oldName, {
      name: newName,
      type: getValidSchemaType(property.schema),
      description:
        typeof property.schema === 'boolean'
          ? ''
          : property.schema.description || '',
      required: property.required,
      validation:
        typeof property.schema === 'boolean'
          ? { type: 'object' }
          : property.schema,
    });
  };

  // Handle required status change
  const handleRequiredChange = (name: string, required: boolean) => {
    const property = properties.find((prop) => prop.name === name);
    if (!property) return;

    onEditField(name, {
      name,
      type: getValidSchemaType(property.schema),
      description:
        typeof property.schema === 'boolean'
          ? ''
          : property.schema.description || '',
      required,
      validation:
        typeof property.schema === 'boolean'
          ? { type: 'object' }
          : property.schema,
    });
  };

  // Handle schema change
  const handleSchemaChange = (
    name: string,
    updatedSchema: ObjectJSONSchema,
  ) => {
    const property = properties.find((prop) => prop.name === name);
    if (!property) return;

    const type = updatedSchema.type || 'object';
    // Ensure we're using a single type, not an array of types
    const validType = Array.isArray(type) ? type[0] || 'object' : type;

    onEditField(name, {
      name,
      type: validType,
      description: updatedSchema.description || '',
      required: property.required,
      validation: updatedSchema,
    });
  };

  const validationTree = useMemo(
    () => buildValidationTree(schema, t),
    [schema, t],
  );

  return (
    <div className="space-y-2 animate-in">
      {properties.map((property) => (
        <SchemaPropertyEditor
          key={property.name}
          name={property.name}
          schema={property.schema}
          required={property.required}
          validationNode={validationTree.children[property.name] ?? undefined}
          onDelete={() => onDeleteField(property.name)}
          onNameChange={(newName) => handleNameChange(property.name, newName)}
          onRequiredChange={(required) =>
            handleRequiredChange(property.name, required)
          }
          onSchemaChange={(schema) => handleSchemaChange(property.name, schema)}
        />
      ))}
    </div>
  );
};

export default SchemaFieldList;
