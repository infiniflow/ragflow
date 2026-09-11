import { Card, CardContent } from '@/components/ui/card';
import { Switch } from '@/components/ui/switch';
import { Annoyed } from 'lucide-react';

interface ParsedPageCardProps {
  page: string;
  content: string;
}

export function ParsedPageCard({ page, content }: ParsedPageCardProps) {
  return (
    <Card className="bg-colors-outline-neutral-standard border-colors-outline-neutral-strong rounded-3xl">
      <CardContent className="p-4">
        <p className="text-colors-text-neutral-standard text-base">{page}</p>
        <div className="text-colors-text-neutral-strong text-lg mt-2">
          {content}
        </div>
      </CardContent>
    </Card>
  );
}

interface ChunkCardProps {
  activated: boolean;
  content: string;
}

export function ChunkCard({ content }: ChunkCardProps) {
  return (
    <Card className="bg-colors-outline-neutral-standard border-colors-outline-neutral-strong rounded-3xl">
      <CardContent className="p-4">
        <div className="flex justify-between items-center">
          <Annoyed />
          <div className="flex items-center space-x-2">
            <Switch />
            <span className="text-colors-text-neutral-strong">Active</span>
          </div>
        </div>
        <div className="text-colors-text-neutral-strong text-lg mt-2 line-clamp-4">
          {content}
        </div>
      </CardContent>
    </Card>
  );
}
