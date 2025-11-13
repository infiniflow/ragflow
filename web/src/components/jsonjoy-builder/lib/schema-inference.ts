import { asObjectSchema, type JSONSchema } from '../types/json-schema';

/**
 * Merges two JSON schemas.
 * If schemas are compatible (e.g., integer and number), attempts to merge.
 * If schemas are identical, returns the first schema.
 * If schemas are incompatible, returns a schema with oneOf.
 */
function mergeSchemas(schema1: JSONSchema, schema2: JSONSchema): JSONSchema {
  const s1 = asObjectSchema(schema1);
  const s2 = asObjectSchema(schema2);

  // Deep comparison for equality
  if (JSON.stringify(s1) === JSON.stringify(s2)) {
    return schema1;
  }

  // Handle basic type merging (e.g., integer into number)
  if (s1.type === 'integer' && s2.type === 'number') return { type: 'number' };
  if (s1.type === 'number' && s2.type === 'integer') return { type: 'number' };

  // If types are different or complex merging is needed, use oneOf
  const existingOneOf = Array.isArray(s1.oneOf) ? s1.oneOf : [s1];
  const newSchemaToAdd = s2;

  // Avoid adding duplicate schemas to oneOf
  if (
    !existingOneOf.some(
      (s) => JSON.stringify(s) === JSON.stringify(newSchemaToAdd),
    )
  ) {
    const mergedOneOf = [...existingOneOf, newSchemaToAdd];
    // Simplify oneOf if it contains only one unique schema after potential merge attempts
    const uniqueSchemas = [
      ...new Map(mergedOneOf.map((s) => [JSON.stringify(s), s])).values(),
    ];
    if (uniqueSchemas.length === 1) {
      return uniqueSchemas[0];
    }
    return { oneOf: uniqueSchemas };
  }

  return s1.oneOf ? s1 : { oneOf: [s1] }; // Return existing oneOf or create new if only s1 existed
}

// --- Helper Functions for Type Inference ---

function inferObjectSchema(obj: Record<string, unknown>): JSONSchema {
  const properties: Record<string, JSONSchema> = {};
  const required: string[] = [];

  for (const [key, value] of Object.entries(obj)) {
    properties[key] = inferSchema(value); // Recursive call
    if (value !== undefined && value !== null) {
      required.push(key);
    }
  }

  return {
    type: 'object',
    properties,
    required: required.length > 0 ? required.sort() : undefined, // Sort required keys
  };
}

function detectEnumsInArrayItems(
  mergedProperties: Record<string, JSONSchema>,
  originalArray: Record<string, unknown>[],
  totalItems: number,
): Record<string, JSONSchema> {
  if (totalItems < 10 || Object.keys(mergedProperties).length === 0) {
    return mergedProperties; // Not enough data or no properties to check
  }

  const valueMap: Record<string, Set<string | number>> = {};

  // Collect distinct values
  for (const item of originalArray) {
    for (const key in mergedProperties) {
      if (Object.prototype.hasOwnProperty.call(item, key)) {
        const value = item[key];
        if (typeof value === 'string' || typeof value === 'number') {
          if (!valueMap[key]) valueMap[key] = new Set();
          valueMap[key].add(value);
        }
      }
    }
  }

  const updatedProperties = { ...mergedProperties };
  // Update schema for properties that look like enums
  for (const key in valueMap) {
    const distinctValues = Array.from(valueMap[key]);
    if (
      distinctValues.length > 1 &&
      distinctValues.length <= 10 &&
      distinctValues.length < totalItems / 2
    ) {
      const currentSchema = asObjectSchema(updatedProperties[key]);
      if (
        currentSchema.type === 'string' ||
        currentSchema.type === 'number' ||
        currentSchema.type === 'integer'
      ) {
        updatedProperties[key] = {
          type: currentSchema.type,
          enum: distinctValues.sort(),
        };
      }
    }
  }
  return updatedProperties;
}

function detectSemanticFormatsInArrayItems(
  mergedProperties: Record<string, JSONSchema>,
  originalArray: Record<string, unknown>[],
): Record<string, JSONSchema> {
  const updatedProperties = { ...mergedProperties };

  for (const key in updatedProperties) {
    const currentSchema = asObjectSchema(updatedProperties[key]);

    // Coordinates Detection
    if (
      /coordinates?|coords?|latLon|lonLat|point/i.test(key) &&
      currentSchema.type === 'array'
    ) {
      const itemsSchema = asObjectSchema(currentSchema.items);
      if (itemsSchema?.type === 'number' || itemsSchema?.type === 'integer') {
        let isValidCoordArray = true;
        let coordLength: number | null = null;
        for (const item of originalArray) {
          if (
            Object.prototype.hasOwnProperty.call(item, key) &&
            Array.isArray(item[key])
          ) {
            const arr = item[key] as unknown[];
            if (coordLength === null) coordLength = arr.length;
            if (
              arr.length !== coordLength ||
              (arr.length !== 2 && arr.length !== 3) ||
              !arr.every((v) => typeof v === 'number')
            ) {
              isValidCoordArray = false;
              break;
            }
          } else if (Object.prototype.hasOwnProperty.call(item, key)) {
            isValidCoordArray = false;
            break;
          }
        }
        if (isValidCoordArray && coordLength !== null) {
          updatedProperties[key] = {
            type: 'array',
            items: { type: 'number' },
            minItems: coordLength,
            maxItems: coordLength,
          };
        }
      }
    }

    // Timestamp Detection
    if (
      /timestamp|createdAt|updatedAt|occurredAt/i.test(key) &&
      currentSchema.type === 'integer'
    ) {
      let isTimestampLike = true;
      const now = Date.now();
      const fiftyYearsAgo = now - 50 * 365 * 24 * 60 * 60 * 1000;
      for (const item of originalArray) {
        if (Object.prototype.hasOwnProperty.call(item, key)) {
          const val = item[key];
          if (
            typeof val !== 'number' ||
            !Number.isInteger(val) ||
            val < fiftyYearsAgo
          ) {
            isTimestampLike = false;
            break;
          }
        }
      }
      if (isTimestampLike) {
        updatedProperties[key] = {
          type: 'integer',
          format: 'unix-timestamp',
          description: 'Unix timestamp (likely milliseconds)',
        };
      }
    }
    // Add more semantic detections here
  }
  return updatedProperties;
}

function processArrayOfObjects(
  itemSchemas: JSONSchema[],
  originalArray: Record<string, unknown>[],
): JSONSchema {
  let mergedProperties: Record<string, JSONSchema> = {};
  const propertyCounts: Record<string, number> = {};
  const totalItems = itemSchemas.length;

  for (const schema of itemSchemas) {
    const objSchema = asObjectSchema(schema);
    if (!objSchema.properties) continue;
    for (const [key, value] of Object.entries(objSchema.properties)) {
      propertyCounts[key] = (propertyCounts[key] || 0) + 1;
      if (key in mergedProperties) {
        mergedProperties[key] = mergeSchemas(mergedProperties[key], value);
      } else {
        mergedProperties[key] = value;
      }
    }
  }

  const requiredProps = Object.entries(propertyCounts)
    .filter(([_, count]) => count === totalItems)
    .map(([key, _]) => key);

  // Apply Enum Detection
  mergedProperties = detectEnumsInArrayItems(
    mergedProperties,
    originalArray,
    totalItems,
  );

  // Apply Semantic Detection
  mergedProperties = detectSemanticFormatsInArrayItems(
    mergedProperties,
    originalArray,
  );

  return {
    type: 'object',
    properties: mergedProperties,
    required: requiredProps.length > 0 ? requiredProps.sort() : undefined,
  };
}

function inferArraySchema(obj: unknown[]): JSONSchema {
  if (obj.length === 0) return { type: 'array', items: {} };

  const itemSchemas = obj.map((item) => inferSchema(item)); // Recursive call

  const firstItemSchema = asObjectSchema(itemSchemas[0]);
  const allSameType = itemSchemas.every(
    (schema) => asObjectSchema(schema).type === firstItemSchema.type,
  );

  if (allSameType) {
    if (firstItemSchema.type === 'object') {
      const itemsSchema = processArrayOfObjects(
        itemSchemas,
        obj as Record<string, unknown>[],
      );
      return {
        type: 'array',
        items: itemsSchema,
        minItems: 0, // Keep minItems consistent
      };
    }
    return {
      type: 'array',
      items: itemSchemas[0],
      minItems: 0,
    };
  }

  // Mixed type arrays
  const uniqueSchemas = [
    ...new Map(itemSchemas.map((s) => [JSON.stringify(s), s])).values(),
  ];

  // Check if merged schemas result in a single object type
  if (
    uniqueSchemas.length === 1 &&
    asObjectSchema(uniqueSchemas[0]).type === 'object'
  ) {
    return {
      type: 'array',
      items: uniqueSchemas[0],
      minItems: 0,
    };
  }

  return {
    type: 'array',
    items:
      uniqueSchemas.length === 1 ? uniqueSchemas[0] : { oneOf: uniqueSchemas },
    minItems: 0,
  };
}

function inferStringSchema(str: string): JSONSchema {
  const formats: Record<string, RegExp> = {
    date: /^\d{4}-\d{2}-\d{2}$/,
    'date-time':
      /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})?$/,
    email: /^[^@]+@[^@]+\.[^@]+$/,
    uuid: /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i,
    uri: /^(https?|ftp):\/\/[^\s/$.?#].[^\s]*$/i,
  };

  for (const [format, regex] of Object.entries(formats)) {
    if (regex.test(str)) {
      return { type: 'string', format };
    }
  }

  return { type: 'string' };
}

function inferNumberSchema(num: number): JSONSchema {
  return Number.isInteger(num) ? { type: 'integer' } : { type: 'number' };
}

// --- Main Inference Function ---

/**
 * Infers a JSON Schema from a JSON object
 * Based on json-schema-generator approach
 */
export function inferSchema(obj: unknown): JSONSchema {
  if (obj === null) return { type: 'null' };

  const type = Array.isArray(obj) ? 'array' : typeof obj;

  switch (type) {
    case 'object':
      return inferObjectSchema(obj as Record<string, unknown>); // Cast needed
    case 'array':
      return inferArraySchema(obj as unknown[]); // Cast needed
    case 'string':
      return inferStringSchema(obj as string);
    case 'number':
      return inferNumberSchema(obj as number);
    case 'boolean':
      return { type: 'boolean' }; // Simple enough to keep inline
    default:
      // Should not happen for valid JSON, but return empty schema as fallback
      return {};
  }
}

/**
 * Creates a full JSON Schema document from a JSON object
 */
export function createSchemaFromJson(jsonObject: unknown): JSONSchema {
  const inferredSchema = inferSchema(jsonObject);

  // Ensure the root schema is always an object, even if input is array/primitive
  const rootSchema = asObjectSchema(inferredSchema);
  const finalSchema: Record<string, unknown> = {
    $schema: 'https://json-schema.org/draft-07/schema',
    title: 'Generated Schema',
    description: 'Generated from JSON data',
  };

  if (rootSchema.type === 'object' || rootSchema.properties) {
    finalSchema.type = 'object';
    finalSchema.properties = rootSchema.properties;
    if (rootSchema.required) finalSchema.required = rootSchema.required;
  } else if (rootSchema.type === 'array' || rootSchema.items) {
    finalSchema.type = 'array';
    finalSchema.items = rootSchema.items;
    if (rootSchema.minItems !== undefined)
      finalSchema.minItems = rootSchema.minItems;
    if (rootSchema.maxItems !== undefined)
      finalSchema.maxItems = rootSchema.maxItems;
  } else if (rootSchema.type) {
    // Handle primitive types at the root (e.g., input is just "hello")
    // This might be less common, but good to handle. Wrap it in an object.
    finalSchema.type = 'object';
    finalSchema.properties = { value: rootSchema };
    finalSchema.required = ['value'];
    finalSchema.title = 'Generated Schema (Primitive Root)';
    finalSchema.description =
      'Input was a primitive value, wrapped in an object.';
  } else {
    // Default empty object if inference fails completely
    finalSchema.type = 'object';
  }

  return finalSchema as JSONSchema;
}
