import Spotlight from '@/components/spotlight';
import { Card, CardContent, CardHeader } from '@/components/ui/card';
import { PropsWithChildren } from 'react';

export function Title({ children }: PropsWithChildren) {
  return <span className="font-bold text-xl">{children}</span>;
}

type ProfileSettingWrapperCardProps = {
  header: React.ReactNode;
} & PropsWithChildren;

export function ProfileSettingWrapperCard({
  header,
  children,
}: ProfileSettingWrapperCardProps) {
  return (
    <Card
      as="article"
      className="relative w-full border-border-button bg-transparent border-0.5 flex flex-col"
    >
      <CardHeader className="flex-0 border-b-0.5 border-border-button p-5">
        {header}
      </CardHeader>

      <CardContent className="flex-1 h-0 p-0">{children}</CardContent>

      <Spotlight />
    </Card>
  );
}
