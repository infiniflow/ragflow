import {
  JSONSchema,
  JsonSchemaVisualizer,
  TranslationProvider,
} from '@/components/jsonjoy-builder';

export function SchemaPanel({ value }: { value: JSONSchema }) {
  return (
    <TranslationProvider>
      <section className="h-48">
        <JsonSchemaVisualizer
          schema={value}
          readOnly
          showHeader={false}
        ></JsonSchemaVisualizer>
      </section>
    </TranslationProvider>
  );
}
