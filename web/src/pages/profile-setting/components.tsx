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
    <Card className="w-full border-border-button bg-transparent relative">
      <CardHeader className="border-b border-border-button p-5">
        {header}
      </CardHeader>
      <CardContent className="p-5">{children}</CardContent>
    </Card>
  );
}
