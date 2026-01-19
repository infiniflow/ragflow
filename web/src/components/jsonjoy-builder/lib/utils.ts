import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';
import type { Translation } from '../i18n/translation-keys.ts';
import type { SchemaType } from '../types/json-schema';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

// Helper functions for backward compatibility
export const getTypeColor = (type: SchemaType): string => {
  switch (type) {
    case 'string':
      return 'text-blue-500 bg-blue-50';
    case 'number':
    case 'integer':
      return 'text-purple-500 bg-purple-50';
    case 'boolean':
      return 'text-green-500 bg-green-50';
    case 'object':
      return 'text-orange-500 bg-orange-50';
    case 'array':
      return 'text-pink-500 bg-pink-50';
    case 'null':
      return 'text-gray-500 bg-gray-50';
  }
};

// Get type display label
export const getTypeLabel = (t: Translation, type: SchemaType): string => {
  switch (type) {
    case 'string':
      return t.schemaTypeString;
    case 'number':
    case 'integer':
      return t.schemaTypeNumber;
    case 'boolean':
      return t.schemaTypeBoolean;
    case 'object':
      return t.schemaTypeObject;
    case 'array':
      return t.schemaTypeArray;
    case 'null':
      return t.schemaTypeNull;
  }
};
