import { formatDate } from '@/utils/date';
import { formatBytes } from '@/utils/file-util';

type Props = {
  size: number;
  name: string;
  create_date: string;
};

export default ({ size, name, create_date }: Props) => {
  const sizeName = formatBytes(size);
  const dateStr = formatDate(create_date);
  return (
    <div>
      <h2 className="text-[16px]">{name}</h2>
      <div className="text-text-secondary text-[12px] pt-[5px]">
        Size：{sizeName} Uploaded Time：{dateStr}
      </div>
    </div>
  );
};
