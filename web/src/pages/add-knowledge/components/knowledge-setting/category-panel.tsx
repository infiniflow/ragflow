import SvgIcon from '@/components/svg-icon';
import { useTranslate } from '@/hooks/common-hooks';
import { useSelectParserList } from '@/hooks/user-setting-hooks';
import { Col, Divider, Empty, Row, Typography } from 'antd';
import DOMPurify from 'dompurify';
import camelCase from 'lodash/camelCase';
import { useMemo } from 'react';
import styles from './index.less';
import { ImageMap } from './utils';

const { Title, Text } = Typography;

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
          <Title level={5} className={styles.topTitle}>
            {`"${item.title}" ${t('methodTitle')}`}
          </Title>
          <p
            dangerouslySetInnerHTML={{
              __html: DOMPurify.sanitize(item.description),
            }}
          ></p>
          <Title level={5}>{`"${item.title}" ${t('methodExamples')}`}</Title>
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
          <Title level={5}>
            {item.title} {t('dialogueExamplesTitle')}
          </Title>
          <Divider></Divider>
        </>
      ) : (
        <Empty description={''} image={null}>
          <p>{t('methodEmpty')}</p>
          <SvgIcon name={'chunk-method/chunk-empty'} width={'100%'}></SvgIcon>
        </Empty>
      )}
    </section>
  );
};

export default CategoryPanel;
