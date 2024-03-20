import { Col, Row, Typography } from 'antd';

import styles from './index.less';

const { Title, Text } = Typography;

const getImageName = (prefix: string, length: number) =>
  Array.of(length).map((x, idx) => `${prefix}-0${idx + 1}.svg`);

const ImageMap = {
  book: Array.of(4).map((x, idx) => `book-0${idx + 1}.svg`),
};

const CategoryPanel = () => {
  return (
    <section className={styles.categoryPanelWrapper}>
      <Title level={5} className={styles.topTitle}>
        Laws Category
      </Title>
      <Text>
        We support files in Word (.docx), PDF (.pdf), and text (.txt) formats.
      </Text>
      <Title level={5}>Laws Category</Title>
      <Text>
        We support files in Word (.docx), PDF (.pdf), and text (.txt) formats.
      </Text>
      <Row>
        <Col span={12}>col-12</Col>
        <Col span={12}>col-12</Col>
      </Row>
    </section>
  );
};

export default CategoryPanel;
