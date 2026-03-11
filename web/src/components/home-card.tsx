import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { formatDate } from '@/utils/date';
import { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

interface IProps {
  data: {
    name: string;
    description?: string;
    avatar?: string;
    update_time?: string | number;
    release_time?: number;
  };
  onClick?: () => void;
  moreDropdown: React.ReactNode;
  sharedBadge?: ReactNode;
  icon?: React.ReactNode;
  testId?: string;
}

function Time({ time }: { time: string | number | undefined }) {
  return (
    <p className="text-sm opacity-80 whitespace-nowrap">{formatDate(time)}</p>
  );
}
export function HomeCard({
  data,
  onClick,
  moreDropdown,
  sharedBadge,
  icon,
  testId,
}: IProps) {
  const { t } = useTranslation();

  return (
    <Card
      as="article"
      data-testid={testId}
      data-agent-name={data.name}
      onClick={() => {
        // navigateToSearch(data?.id);
        onClick?.();
      }}
      tabIndex={0}
      className="px-2.5 py-4 flex gap-2 items-start group h-full w-full hover:shadow-md"
    >
      <div>
        <RAGFlowAvatar
          className="w-[32px] h-[32px]"
          avatar={data.avatar}
          name={data.name}
        />
      </div>

      <div className="flex-1 w-0">
        <CardHeader
          as="header"
          className="p-0 flex-1 flex flex-row items-center gap-2 space-y-0"
        >
          <CardTitle className="flex-1 inline-flex w-0 me-auto">
            <h3
              className="flex-1 truncate text-base font-bold leading-snug"
              data-testid="agent-name"
            >
              {data.name}
            </h3>

            {icon}
          </CardTitle>

          <div>{moreDropdown}</div>
        </CardHeader>

        <CardContent className="p-0">
          <div className="flex flex-col justify-between gap-1 flex-1 h-full w-[calc(100%-50px)]">
            <section className="flex justify-between"></section>

            <section className="flex flex-col gap-1 mt-1">
              <div className="whitespace-nowrap overflow-hidden text-ellipsis">
                {data.description}
              </div>
              <div className="flex justify-between items-center">
                {data.release_time ? (
                  <section>
                    <div className="flex items-center gap-2 text-sm opacity-80">
                      {`${t('flow.lastSavedAt')}:`}
                      <Time time={data.update_time}></Time>
                    </div>
                    <div className="flex items-center gap-2 text-sm opacity-80">
                      {`${t('flow.publishedAt')}:`}
                      <Time time={data.release_time}></Time>
                    </div>
                  </section>
                ) : (
                  <Time time={data.update_time}></Time>
                )}
                {sharedBadge}
              </div>
            </section>
          </div>
        </CardContent>
      </div>
    </Card>
  );
}
