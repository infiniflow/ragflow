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

import { formatDate } from '@/utils/date';
import { formatBytes } from '@/utils/file-util';
import { PropsWithChildren } from 'react';
import { useTranslation } from 'react-i18next';

type Props = {
  size: number;
  name: string;
  create_date: string;
  className?: string;
  wrapperClassName?: string;
};

export default function DocumentHeader({
  size,
  name,
  create_date,
  className,
  children,
  wrapperClassName,
}: PropsWithChildren<Props>) {
  const sizeName = formatBytes(size);
  const dateStr = formatDate(create_date);

  const { t } = useTranslation();

  return (
    <header className={wrapperClassName}>
      <section className={className}>
        <h2 className="text-2xl font-semibold truncate">{name}</h2>
        <dl
          className="
          text-text-secondary text-sm flex truncate
          [&_dt]:after:content-[':'] [&_dt]:after:me-[.5ch]
          [&_dd]:me-[2ch]"
        >
          <dt>{t('chunk.size')}</dt>
          <dd>{sizeName}</dd>

          <dt>{t('chunk.uploadedTime')}</dt>
          <dd>{dateStr}</dd>
        </dl>
      </section>
      {children}
    </header>
  );
}
