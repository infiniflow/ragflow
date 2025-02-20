import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { useSetModalState } from '@/hooks/common-hooks';
import { IFlowTemplate } from '@/interfaces/database/flow';
import { useTranslation } from 'react-i18next';

interface IProps {
  data: IFlowTemplate;
}

export function TemplateCard({
  data,
  showModal,
}: IProps & Pick<ReturnType<typeof useSetModalState>, 'showModal'>) {
  const { t } = useTranslation();
  return (
    <Card className="bg-colors-background-inverse-weak  border-colors-outline-neutral-standard group relative">
      <CardContent className="p-4 ">
        <div className="flex justify-between mb-4">
          {data.avatar ? (
            <div
              className="w-[70px] h-[70px] rounded-xl bg-cover"
              style={{ backgroundImage: `url(${data.avatar})` }}
            />
          ) : (
            <Avatar className="w-[70px] h-[70px]">
              <AvatarImage src="https://github.com/shadcn.png" />
              <AvatarFallback>CN</AvatarFallback>
            </Avatar>
          )}
        </div>
        <h3 className="text-xl font-bold mb-2">{data.title}</h3>
        <p className="break-words">{data.description}</p>
        <Button
          variant="tertiary"
          className="absolute bottom-4 right-4 left-4 hidden justify-end group-hover:block text-center"
          onClick={showModal}
        >
          {t('flow.useTemplate')}
        </Button>
      </CardContent>
    </Card>
  );
}
