import { Card, CardContent } from '@/components/ui/card';
import { IMCPTool } from '@/interfaces/database/mcp';

export type McpToolCardProps = {
  data: IMCPTool;
};

export function McpToolCard({ data }: McpToolCardProps) {
  return (
    <Card>
      <CardContent className="p-2.5 pt-2 group">
        <h3 className="text-sm font-semibold line-clamp-1 pb-2">{data.name}</h3>
        <div className="text-xs font-normal mb-3 text-text-secondary">
          {data.description}
        </div>
      </CardContent>
    </Card>
  );
}
