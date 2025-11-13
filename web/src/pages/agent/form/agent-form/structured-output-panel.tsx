import { JSONSchema, JsonSchemaVisualizer } from '@/components/jsonjoy-builder';

export function StructuredOutputPanel({ value }: { value: JSONSchema }) {
  return (
    <section className="h-48">
      <JsonSchemaVisualizer
        schema={value}
        readOnly
        showHeader={false}
      ></JsonSchemaVisualizer>
    </section>
  );
}
