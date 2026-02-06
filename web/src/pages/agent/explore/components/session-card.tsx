import { Card, CardContent } from '@/components/ui/card';
import { IAgentLogResponse } from '@/interfaces/database/agent';
import { cn } from '@/lib/utils';

interface SessionCardProps {
  session: IAgentLogResponse;
  selected?: boolean;
  onClick: () => void;
}

export function SessionCard({ session, selected, onClick }: SessionCardProps) {
  const firstUserMessage = session.message?.find((msg) => msg.role === 'user');
  const displayName =
    firstUserMessage?.content?.slice(0, 50) ||
    `Session ${session.id.slice(0, 8)}`;

  return (
    <Card
      onClick={onClick}
      className={cn(
        'cursor-pointer hover:shadow-md transition-shadow',
        selected && 'bg-bg-card',
      )}
    >
      <CardContent className="p-3">
        <div className="text-sm font-medium truncate">{displayName}</div>
        <div className="text-xs text-text-secondary mt-1">
          {session.round} messages •{' '}
          {new Date(session.create_time * 1000).toLocaleDateString()}
        </div>
      </CardContent>
    </Card>
  );
}
