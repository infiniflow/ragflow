import { ReactComponent as FilterIcon } from '@/assets/filter.svg';
import { PlusOutlined } from '@ant-design/icons';
import { Button, Col, Row, Space } from 'antd';
import { useCallback, useEffect } from 'react';
import { useDispatch, useNavigate, useSelector } from 'umi';
import styles from './index.less';
import KnowledgeCard from './knowledge-card';

const Knowledge = () => {
  const dispatch = useDispatch();
  const knowledgeModel = useSelector((state: any) => state.knowledgeModel);
  const navigate = useNavigate();
  const { data = [] } = knowledgeModel;

  const fetchList = useCallback(() => {
    dispatch({
      type: 'knowledgeModel/getList',
      payload: {},
    });
  }, []);

  const handleAddKnowledge = () => {
    navigate(`add/setting?activeKey=setting`);
  };

  useEffect(() => {
    fetchList();
  }, [fetchList]);

  return (
    <div className={styles.knowledge}>
      <div className={styles.topWrapper}>
        <div>
          <span className={styles.title}>Welcome back, Zing</span>
          <p className={styles.description}>
            Which database are we going to use today?
          </p>
        </div>
        <Space size={'large'}>
          <Button icon={<FilterIcon />} className={styles.filterButton}>
            Filters
          </Button>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={handleAddKnowledge}
            className={styles.topButton}
          >
            Create knowledge base
          </Button>
        </Space>
      </div>
      <Row gutter={{ xs: 8, sm: 16, md: 24, lg: 32 }}>
        {data.map((item: any) => {
          return (
            <Col
              className="gutter-row"
              key={item.name}
              xs={24}
              sm={12}
              md={8}
              lg={6}
            >
              <KnowledgeCard item={item}></KnowledgeCard>
            </Col>
          );
        })}
      </Row>
    </div>
  );
};

export default Knowledge;
