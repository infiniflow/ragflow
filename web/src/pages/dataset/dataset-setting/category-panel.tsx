import SvgIcon from '@/components/svg-icon';
import Divider from '@/components/ui/divider';
import { useTranslate } from '@/hooks/common-hooks';
import { useSelectParserList } from '@/hooks/use-user-setting-request';
import DOMPurify from 'dompurify';
import camelCase from 'lodash/camelCase';
import { useMemo } from 'react';
import { TagTabs } from './tag-tabs';
import { ImageMap } from './utils';

const CategoryPanel = ({ chunkMethod }: { chunkMethod: string }) => {
  const parserList = useSelectParserList();
  const { t } = useTranslate('knowledgeConfiguration');

  const item = useMemo(() => {
    const item = parserList.find((x) => x.value === chunkMethod);
    if (item) {
      return {
        title: item.label,
        description: t(camelCase(item.value)),
      };
    }
    return { title: '', description: '' };
  }, [parserList, chunkMethod, t]);

  const imageList = useMemo(() => {
    if (chunkMethod in ImageMap) {
      return ImageMap[chunkMethod as keyof typeof ImageMap];
    }
    return [];
  }, [chunkMethod]);

  return (
    <section>
      {imageList.length > 0 ? (
        <>
          <h5 className="font-semibold text-base mt-0 mb-1">
            {`"${item.title}" ${t('methodTitle')}`}
          </h5>
          <p
            dangerouslySetInnerHTML={{
              __html: DOMPurify.sanitize(item.description),
            }}
          ></p>
          <h5 className="font-semibold text-base mt-4 mb-1">{`"${item.title}" ${t('methodExamples')}`}</h5>
          <span className="text-text-secondary">
            {t('methodExamplesDescription')}
          </span>
          <div className="grid grid-cols-2 gap-2.5 mt-4">
            {imageList.map((x) => (
              <SvgIcon
                name={x}
                width={'100%'}
                className="w-full"
                key={x}
              ></SvgIcon>
            ))}
          </div>
          <h5 className="font-semibold text-base mt-4 mb-1">
            {item.title} {t('dialogueExamplesTitle')}
          </h5>
          <Divider></Divider>
        </>
      ) : (
        <div className="flex flex-col items-center justify-center py-8">
          <p className="text-text-secondary mb-4">{t('methodEmpty')}</p>
          <SvgIcon name={'chunk-method/chunk-empty'} width={'100%'}></SvgIcon>
        </div>
      )}
      {chunkMethod === 'tag' && <TagTabs></TagTabs>}
    </section>
  );
};

export default CategoryPanel;
