// https://github.com/lovasoa/jsonjoy-builder v0.1.0
// exports for public API

import JsonSchemaEditor, {
  type JsonSchemaEditorProps,
} from './components/schema-editor/json-schema-editor';
import JsonSchemaVisualizer, {
  type JsonSchemaVisualizerProps,
} from './components/schema-editor/json-schema-visualizer';
import SchemaVisualEditor, {
  type SchemaVisualEditorProps,
} from './components/schema-editor/schema-visual-editor';

export * from './i18n/locales/de';
export * from './i18n/locales/en';
export * from './i18n/translation-context';
export * from './i18n/translation-keys';

export * from './components/features/json-validator';
export * from './components/features/schema-inferencer';

export {
  JsonSchemaEditor,
  JsonSchemaVisualizer,
  SchemaVisualEditor,
  type JsonSchemaEditorProps,
  type JsonSchemaVisualizerProps,
  type SchemaVisualEditorProps,
};

export type { JSONSchema, baseSchema } from './types/json-schema';
