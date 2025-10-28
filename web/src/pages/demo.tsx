import { JSONSchema, SchemaVisualEditor } from '@/components/jsonjoy-builder';
import { useState } from 'react';

export default function Demo() {
  const [schema, setSchema] = useState<JSONSchema>({});
  return (
    <div>
      <h1>JSONJoy Builder</h1>
      <SchemaVisualEditor schema={schema} onChange={setSchema} />
    </div>
  );
}
