import { Button, Flex, Space } from 'antd';

import { useFetchFlow } from '@/hooks/flow-hooks';
import { ArrowLeftOutlined } from '@ant-design/icons';
import { Link } from 'umi';
import { useSaveGraph } from '../hooks';

import styles from './index.less';

interface IProps {
  showChatDrawer(): void;
}

const FlowHeader = ({ showChatDrawer }: IProps) => {
  const { saveGraph } = useSaveGraph();

  const { data } = useFetchFlow();

  return (
    <>
      <Flex
        align="center"
        justify={'space-between'}
        gap={'large'}
        className={styles.flowHeader}
      >
        <Space size={'large'}>
          <Link to={`/flow`}>
            <ArrowLeftOutlined />
          </Link>
          <h3>{data.title}</h3>
        </Space>
        <Space size={'large'}>
          <Button onClick={showChatDrawer}>
            <b>Debug</b>
          </Button>
          <Button type="primary" onClick={saveGraph}>
            <b>Save</b>
          </Button>
        </Space>
      </Flex>
    </>
  );
};

export default FlowHeader;
