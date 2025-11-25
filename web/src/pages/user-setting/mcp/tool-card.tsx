import { IMCPTool } from '@/interfaces/database/mcp';

export type McpToolCardProps = {
  data: IMCPTool;
};

export function McpToolCard({ data }: McpToolCardProps) {
  return (
    <section className="group py-2.5">
      <div className="text-sm font-normal line-clamp-1 pb-2">{data.name}</div>
      <div className="text-xs font-normal text-text-secondary">
        {data.description}
      </div>
    </section>
  );
}
