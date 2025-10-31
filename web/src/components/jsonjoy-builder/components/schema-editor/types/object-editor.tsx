import { useTranslation } from '../../../hooks/use-translation';
import {
  getSchemaProperties,
  removeObjectProperty,
  updateObjectProperty,
  updatePropertyRequired,
} from '../../../lib/schema-editor';
import type { NewField, ObjectJSONSchema } from '../../../types/json-schema';
import { asObjectSchema, isBooleanSchema } from '../../../types/json-schema';
import AddFieldButton from '../add-field-button';
import SchemaPropertyEditor from '../schema-property-editor';
import type { TypeEditorProps } from '../type-editor';

const ObjectEditor: React.FC<TypeEditorProps> = ({
  schema,
  validationNode,
  onChange,
  depth = 0,
}) => {
  const t = useTranslation();

  // Get object properties
  const properties = getSchemaProperties(schema);

  // Create a normalized schema object
  const normalizedSchema: ObjectJSONSchema = isBooleanSchema(schema)
    ? { type: 'object', properties: {} }
    : { ...schema, type: 'object', properties: schema.properties || {} };

  // Handle adding a new property
  const handleAddProperty = (newField: NewField) => {
    // Create field schema from the new field data
    const fieldSchema = {
      type: newField.type,
      description: newField.description || undefined,
      ...(newField.validation || {}),
    } as ObjectJSONSchema;

    // Add the property to the schema
    let newSchema = updateObjectProperty(
      normalizedSchema,
      newField.name,
      fieldSchema,
    );

    // Update required status if needed
    if (newField.required) {
      newSchema = updatePropertyRequired(newSchema, newField.name, true);
    }

    // Update the schema
    onChange(newSchema);
  };

  // Handle deleting a property
  const handleDeleteProperty = (propertyName: string) => {
    const newSchema = removeObjectProperty(normalizedSchema, propertyName);
    onChange(newSchema);
  };

  // Handle property name change
  const handlePropertyNameChange = (oldName: string, newName: string) => {
    if (oldName === newName) return;

    const property = properties.find((p) => p.name === oldName);
    if (!property) return;

    const propertySchemaObj = asObjectSchema(property.schema);

    // Add property with new name
    let newSchema = updateObjectProperty(
      normalizedSchema,
      newName,
      propertySchemaObj,
    );

    if (property.required) {
      newSchema = updatePropertyRequired(newSchema, newName, true);
    }

    newSchema = removeObjectProperty(newSchema, oldName);

    onChange(newSchema);
  };

  // Handle property required status change
  const handlePropertyRequiredChange = (
    propertyName: string,
    required: boolean,
  ) => {
    const newSchema = updatePropertyRequired(
      normalizedSchema,
      propertyName,
      required,
    );
    onChange(newSchema);
  };

  const handlePropertySchemaChange = (
    propertyName: string,
    propertySchema: ObjectJSONSchema,
  ) => {
    const newSchema = updateObjectProperty(
      normalizedSchema,
      propertyName,
      propertySchema,
    );
    onChange(newSchema);
  };

  return (
    <div className="space-y-4">
      {properties.length > 0 ? (
        <div className="space-y-2">
          {properties.map((property) => (
            <SchemaPropertyEditor
              key={property.name}
              name={property.name}
              schema={property.schema}
              required={property.required}
              validationNode={validationNode?.children[property.name]}
              onDelete={() => handleDeleteProperty(property.name)}
              onNameChange={(newName) =>
                handlePropertyNameChange(property.name, newName)
              }
              onRequiredChange={(required) =>
                handlePropertyRequiredChange(property.name, required)
              }
              onSchemaChange={(schema) =>
                handlePropertySchemaChange(property.name, schema)
              }
              depth={depth}
            />
          ))}
        </div>
      ) : (
        <div className="text-sm text-muted-foreground italic p-2 text-center border rounded-md">
          {t.objectPropertiesNone}
        </div>
      )}

      <div className="mt-4">
        <AddFieldButton onAddField={handleAddProperty} variant="secondary" />
      </div>
    </div>
  );
};

export default ObjectEditor;
