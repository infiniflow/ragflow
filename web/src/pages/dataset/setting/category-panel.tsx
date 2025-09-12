import SvgIcon from '@/components/svg-icon';
import { useTranslate } from '@/hooks/common-hooks';
import { useSelectParserList } from '@/hooks/user-setting-hooks';
import { Col, Divider, Empty, Row, Typography } from 'antd';
import DOMPurify from 'dompurify';
import camelCase from 'lodash/camelCase';
import { useMemo } from 'react';
import styles from './index.less';
import { TagTabs } from './tag-tabs';
import { ImageMap } from './utils';

const { Text } = Typography;

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
    <section className={styles.categoryPanelWrapper}>
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
          <Text>{t('methodExamplesDescription')}</Text>
          <Row gutter={[10, 10]} className={styles.imageRow}>
            {imageList.map((x) => (
              <Col span={12} key={x}>
                <SvgIcon
                  name={x}
                  width={'100%'}
                  className={styles.image}
                ></SvgIcon>
              </Col>
            ))}
          </Row>
          <h5 className="font-semibold text-base mt-4 mb-1">
            {item.title} {t('dialogueExamplesTitle')}
          </h5>
          <Divider></Divider>
        </>
      ) : (
        <Empty description={''} image={null}>
          <p>{t('methodEmpty')}</p>
          <SvgIcon name={'chunk-method/chunk-empty'} width={'100%'}></SvgIcon>
        </Empty>
      )}
      {chunkMethod === 'tag' && <TagTabs></TagTabs>}
    </section>
  );
};

export default CategoryPanel;
