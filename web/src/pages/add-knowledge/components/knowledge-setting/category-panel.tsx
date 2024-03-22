import SvgIcon from '@/components/svg-icon';
import { useSelectParserList } from '@/hooks/userSettingHook';
import { Col, Divider, Empty, Row, Typography } from 'antd';
import { useMemo } from 'react';
import styles from './index.less';
import { ImageMap, TextMap } from './utils';

const { Title, Text } = Typography;

const CategoryPanel = ({ chunkMethod }: { chunkMethod: string }) => {
  const parserList = useSelectParserList();

  const item = useMemo(() => {
    const item = parserList.find((x) => x.value === chunkMethod);
    if (item) {
      return {
        title: item.label,
        description: TextMap[item.value as keyof typeof TextMap]?.description,
      };
    }
    return { title: '', description: '' };
  }, [parserList, chunkMethod]);

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
            "{item.title}" Chunking Method Description
          </Title>
          <p
            dangerouslySetInnerHTML={{
              __html: item.description,
            }}
          ></p>
          <Title level={5}>"{item.title}" Examples</Title>
          <Text>
            This visual guides is in order to make understanding easier
            for you.
          </Text>
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
          <Title level={5}>{item.title} Dialogue Examples</Title>
          <Divider></Divider>
        </>
      ) : (
        <Empty description={''} image={null}>
          <p>
            This will display a visual explanation of the knowledge base
            categories
          </p>
          <SvgIcon name={'chunk-method/chunk-empty'} width={'100%'}></SvgIcon>
        </Empty>
      )}
    </section>
  );
};

export default CategoryPanel;
