import { Card, CardContent } from '@/components/ui/card';
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
          <div className="pb-3 text-sm">{data.name}</div>
          <p className="text-text-secondary text-xs">{data.description}</p>
        </section>
      </CardContent>
    </Card>
  );
}
