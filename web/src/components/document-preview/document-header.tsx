import { formatDate } from '@/utils/date';
import { formatBytes } from '@/utils/file-util';
import { useTranslation } from 'react-i18next';

type Props = {
  size: number;
  name: string;
  create_date: string;
  className?: string;
};

export default function DocumentHeader({
  size,
  name,
  create_date,
  className,
}: Props) {
  const sizeName = formatBytes(size);
  const dateStr = formatDate(create_date);

  const { t } = useTranslation();

  return (
    <header className={className}>
      <h2 className="text-2xl font-semibold truncate">{name}</h2>
      <dl
        className="
        text-text-secondary text-sm flex
        [&_dt]:after:content-[':'] [&_dt]:after:me-[.5ch]
        [&_dd]:me-[2ch]"
      >
        <dt>{t('chunk.size')}</dt>
        <dd>{sizeName}</dd>

        <dt>{t('chunk.uploadedTime')}</dt>
        <dd>{dateStr}</dd>
      </dl>
    </header>
  );
}
