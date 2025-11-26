import { Card, CardContent, CardHeader } from '@/components/ui/card';
import { PropsWithChildren } from 'react';

export const UserSettingHeader = ({
  name,
  description,
}: {
  name: string;
  description?: string;
}) => {
  return (
    <>
      <header className="flex flex-col gap-1.5 justify-between items-start p-0">
        <div className="text-2xl font-medium text-text-primary">{name}</div>
        {description && (
          <div className="text-sm text-text-secondary ">{description}</div>
        )}
      </header>
      {/* <Separator className="border-border-button bg-border-button h-[0.5px]" /> */}
    </>
  );
};

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
    <Card className="w-full border-border-button bg-transparent relative border-[0.5px]">
      <CardHeader className="border-b-[0.5px] border-border-button p-5 ">
        {header}
      </CardHeader>
      <CardContent className="p-5">{children}</CardContent>
    </Card>
  );
}
