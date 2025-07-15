import { Card, CardContent, CardTitle } from '@/components/ui/card';
import { IMCPTool } from '@/interfaces/database/mcp';
import { PropsWithChildren } from 'react';

export function MCPCard({
  data,
  children,
}: { data: IMCPTool } & PropsWithChildren) {
  return (
    <Card className="p-3">
      <CardContent className="p-0 flex gap-3">
        {children}
        <section>
          <CardTitle className="pb-3">{data.name}</CardTitle>
          <p>{data.description}</p>
        </section>
      </CardContent>
    </Card>
  );
}
