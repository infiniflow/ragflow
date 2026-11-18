import SvgIcon from '@/components/svg-icon';
import Divider from '@/components/ui/divider';
import { useSelectParserList } from '@/hooks/use-user-setting-request';
import DOMPurify from 'dompurify';
import camelCase from 'lodash/camelCase';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

// A few images in the repo (infiniflow-ai/ragflow-images, builtin/chunk-method)
// are jpg rather than png; override them by 1-based index.
const getImageName = (
  prefix: string,
  length: number,
  extensionOverrides: Record<number, string> = {},
) =>
  new Array(length)
    .fill(0)
    .map(
      (_x, idx) =>
        `https://raw.gitcode.com/infiniflow-ai/ragflow-images/raw/main/builtin/chunk-method/${prefix}-0${idx + 1}.${extensionOverrides[idx + 1] ?? 'png'}`,
    );

// The Go pipeline catalog uses 'general' as the id of the parser whose
// description lives under the 'naive' locale key.
const DescriptionKeyMap: Record<string, string> = {
  general: 'naive',
};

const ImageMap = {
  audio: getImageName('audio', 1),
  email: getImageName('email', 1),
  book: getImageName('book', 4, { 1: 'jpg' }),
  laws: getImageName('law', 2),
  manual: getImageName('manual', 4),
  picture: getImageName('media', 2),
  naive: getImageName('naive', 2, { 2: 'jpg' }),
  general: getImageName('naive', 2, { 2: 'jpg' }),
  paper: getImageName('paper', 2),
  presentation: getImageName('presentation', 2),
  qa: getImageName('qa', 2),
  resume: getImageName('resume', 2),
  table: getImageName('table', 2, { 1: 'jpg' }),
  one: getImageName('one', 2),
  tag: getImageName('tag', 2),
};

const ChunkMethodScreenshot = ({ src }: { src: string }) => {
  const { t } = useTranslation();
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    setFailed(false);
  }, [src]);

  const handleError = useCallback(() => setFailed(true), []);

  if (failed) {
    return (
      <div className="flex h-40 w-full items-center justify-center rounded-md border border-border-button bg-bg-card">
        <span className="text-sm text-text-secondary">
          {t('knowledgeConfiguration.imageLoadFailed')}
        </span>
      </div>
    );
  }

  return (
    <img
      src={src}
      alt=""
      width={'100%'}
      className="w-full max-w-full"
      onError={handleError}
    />
  );
};

const CategoryPanel = ({ chunkMethod }: { chunkMethod: string }) => {
  const parserList = useSelectParserList();
  const { t } = useTranslation();

  const item = useMemo(() => {
    const item = parserList.find((x) => x.value === chunkMethod);
    if (item) {
      const descriptionKey = DescriptionKeyMap[item.value] ?? item.value;
      return {
        title: item.label,
        // Methods without a description entry get an empty string, so the
        // empty placeholder below is still shown for them.
        description: t(`knowledgeConfiguration.${camelCase(descriptionKey)}`, {
          defaultValue: '',
        }),
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

  const hasDescription = item.description.trim().length > 0;

  return (
    <div>
      {hasDescription ? (
        <>
          <h5 className="font-semibold text-base mt-0 mb-1">
            {`"${item.title}" ${t('knowledgeConfiguration.methodTitle')}`}
          </h5>
          <p
            className="[&_ul]:list-disc [&_ol]:list-decimal [&_:is(ul,ol)]:pl-8"
            dangerouslySetInnerHTML={{
              __html: DOMPurify.sanitize(item.description),
            }}
          ></p>
          {imageList.length > 0 && (
            <>
              <h5 className="font-semibold text-base mt-4 mb-1">{`"${item.title}" ${t('knowledgeConfiguration.methodExamples')}`}</h5>
              <span className="text-text-secondary">
                {t('knowledgeConfiguration.methodExamplesDescription')}
              </span>
              <div className="grid grid-cols-2 gap-2.5 mt-4">
                {imageList.map((x) => (
                  <ChunkMethodScreenshot key={x} src={x} />
                ))}
              </div>
              <h5 className="font-semibold text-base mt-4 mb-1">
                {item.title} {t('knowledgeConfiguration.dialogueExamplesTitle')}
              </h5>
              <Divider></Divider>
            </>
          )}
        </>
      ) : (
        <div className="flex flex-col items-center justify-center py-8">
          <p className="text-text-secondary mb-4">
            {t('knowledgeConfiguration.methodEmpty')}
          </p>
          <SvgIcon name={'chunk-method/chunk-empty'} width={'100%'}></SvgIcon>
        </div>
      )}
    </div>
  );
};

export default CategoryPanel;
