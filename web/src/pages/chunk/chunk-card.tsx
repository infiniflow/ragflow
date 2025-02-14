import { Card, CardContent } from '@/components/ui/card';

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
}

export function ChunkCard({}: ChunkCardProps) {
  return (
    <Card className="bg-colors-outline-neutral-standard border-colors-outline-neutral-strong rounded-3xl">
      <CardContent className="p-4">
        <p className="text-colors-text-neutral-standard text-base">{}</p>
        <div className="text-colors-text-neutral-strong text-lg mt-2">{}</div>
      </CardContent>
    </Card>
  );
}
