// https://github.com/lovasoa/jsonjoy-builder v0.1.0
// exports for public API

import JsonSchemaEditor, {
  type JsonSchemaEditorProps,
} from './components/SchemaEditor/JsonSchemaEditor';
import JsonSchemaVisualizer, {
  type JsonSchemaVisualizerProps,
} from './components/SchemaEditor/JsonSchemaVisualizer';
import SchemaVisualEditor, {
  type SchemaVisualEditorProps,
} from './components/SchemaEditor/SchemaVisualEditor';

export * from './i18n/locales/de';
export * from './i18n/locales/en';
export * from './i18n/translation-context';
export * from './i18n/translation-keys';

export * from './components/features/JsonValidator';
export * from './components/features/SchemaInferencer';

export {
  JsonSchemaEditor,
  JsonSchemaVisualizer,
  SchemaVisualEditor,
  type JsonSchemaEditorProps,
  type JsonSchemaVisualizerProps,
  type SchemaVisualEditorProps,
};

export type { JSONSchema, baseSchema } from './types/jsonSchema.ts';
