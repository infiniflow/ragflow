import { ReactComponent as SelectedFilesCollapseIcon } from '@/assets/svg/selected-files-collapse.svg';
import { Card, Collapse, Flex, Space } from 'antd';
import SelectFiles from './select-files';

import styles from './index.less';

const list = [1, 2, 3, 4];

const TestingResult = () => {
  return (
    <section className={styles.testingResultWrapper}>
      <Collapse
        expandIcon={() => (
          <SelectedFilesCollapseIcon></SelectedFilesCollapseIcon>
        )}
        className={styles.selectFilesCollapse}
        items={[
          {
            key: '1',
            label: (
              <Flex
                justify={'space-between'}
                align="center"
                className={styles.selectFilesTitle}
              >
                <span>4/25 Files Selected</span>
                <Space size={52}>
                  <b>Hits</b>
                  <b>View</b>
                </Space>
              </Flex>
            ),
            children: (
              <div>
                <SelectFiles></SelectFiles>
              </div>
            ),
          },
        ]}
      />
      <Flex gap={'large'} vertical>
        {list.map((x) => (
          <Card key={x} title="Default size card" extra={<a href="#">More</a>}>
            <p>Card content</p>
            <p>Card content</p>
            <p>Card content</p>
          </Card>
        ))}
      </Flex>
    </section>
  );
};

export default TestingResult;
