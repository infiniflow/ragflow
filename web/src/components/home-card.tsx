/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { TruncatedText } from '@/components/truncated-text';
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
  showReleaseTime?: boolean;
  extra?: ReactNode;
}

function Time({ time }: { time: string | number | undefined }) {
  return <p className="text-sm truncate">{formatDate(time)}</p>;
}

export function HomeCard({
  data,
  onClick,
  moreDropdown,
  sharedBadge,
  icon,
  testId,
  showReleaseTime = false,
  extra,
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
            <TruncatedText
              as="h3"
              className="flex-1 truncate text-base font-bold leading-snug"
              testId="agent-name"
              tooltip={data.name}
            >
              {data.name}
            </TruncatedText>

            {icon}
          </CardTitle>

          <div>{moreDropdown}</div>
        </CardHeader>

        <CardContent className="p-0">
          <div className="flex flex-col justify-between gap-1 flex-1 h-full w-[calc(100%-50px)]">
            <section className="flex justify-between"></section>

            <section className="flex flex-col gap-1 mt-1">
              <TruncatedText
                className="whitespace-nowrap overflow-hidden text-ellipsis"
                tooltip={data.description}
              >
                {data.description}
              </TruncatedText>
              {extra}
              <div className="flex justify-between items-center min-w-0">
                {showReleaseTime ? (
                  <section className="text-sm text-text-secondary space-y-1 min-w-0">
                    <div className="flex items-center gap-2 min-w-0">
                      <span className="whitespace-nowrap">
                        {t('flow.lastSavedAt')}:
                      </span>
                      <Time time={data.update_time}></Time>
                    </div>
                    {data.release_time && (
                      <div className="flex items-center gap-2 min-w-0">
                        <span className="whitespace-nowrap">
                          {t('flow.publishedAt')}:
                        </span>
                        <Time time={data.release_time}></Time>
                      </div>
                    )}
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
