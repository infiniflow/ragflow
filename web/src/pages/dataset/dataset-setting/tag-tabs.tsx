import { Segmented, SegmentedLabeledOption } from '@/components/ui/segmented';
import { upperFirst } from 'lodash';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { TagTable } from './tag-table';
import { TagWordCloud } from './tag-word-cloud';

enum TagType {
  Cloud = 'cloud',
  Table = 'table',
}

const TagContentMap = {
  [TagType.Cloud]: <TagWordCloud></TagWordCloud>,
  [TagType.Table]: <TagTable></TagTable>,
};

export function TagTabs() {
  const [value, setValue] = useState<TagType>(TagType.Cloud);
  const { t } = useTranslation();

  const options: SegmentedLabeledOption[] = [TagType.Cloud, TagType.Table].map(
    (x) => ({
      label: t(`knowledgeConfiguration.tag${upperFirst(x)}`),
      value: x,
    }),
  );

  return (
    <section className="mt-4">
      <Segmented
        className="w-fit"
        value={value}
        options={options}
        onChange={(val) => setValue(val as TagType)}
      />
      {TagContentMap[value]}
    </section>
  );
}
