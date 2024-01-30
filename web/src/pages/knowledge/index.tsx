import { formatDate } from '@/utils/date';
import {
  DeleteOutlined,
  MinusSquareOutlined,
  PlusOutlined,
} from '@ant-design/icons';
import { Card, Col, FloatButton, Popconfirm, Row } from 'antd';
import { useCallback, useEffect } from 'react';
import { useDispatch, useNavigate, useSelector } from 'umi';
import styles from './index.less';

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

  const confirm = (id: string) => {
    dispatch({
      type: 'knowledgeModel/rmKb',
      payload: {
        kb_id: id,
      },
    });
  };
  const handleAddKnowledge = () => {
    navigate(`add/setting?activeKey=setting`);
  };
  const handleEditKnowledge = (id: string) => {
    navigate(`add/setting?activeKey=file&id=${id}`);
  };
  useEffect(() => {
    fetchList();
  }, [fetchList]);
  return (
    <>
      <div className={styles.knowledge}>
        <FloatButton
          onClick={handleAddKnowledge}
          icon={<PlusOutlined />}
          type="primary"
          style={{ right: 24, top: 100 }}
        />
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
                <Card
                  className={styles.card}
                  onClick={() => {
                    handleEditKnowledge(item.id);
                  }}
                >
                  <div className={styles.container}>
                    <div className={styles.content}>
                      <span className={styles.context}>{item.name}</span>
                      <span className={styles.delete}>
                        <Popconfirm
                          title="Delete the task"
                          description="Are you sure to delete this task?"
                          onConfirm={(e: any) => {
                            e.stopPropagation();
                            e.nativeEvent.stopImmediatePropagation();
                            confirm(item.id);
                          }}
                          okText="Yes"
                          cancelText="No"
                        >
                          <DeleteOutlined
                            onClick={(e) => {
                              e.stopPropagation();
                              e.nativeEvent.stopImmediatePropagation();
                            }}
                          />
                        </Popconfirm>
                      </span>
                    </div>
                    <div className={styles.footer}>
                      <span className={styles.text}>
                        <MinusSquareOutlined />
                        {item.doc_num}文档
                      </span>
                      <span className={styles.text}>
                        <MinusSquareOutlined />
                        {item.chunk_num}个
                      </span>
                      <span className={styles.text}>
                        <MinusSquareOutlined />
                        {item.token_num}千字符
                      </span>
                      <span style={{ float: 'right' }}>
                        {formatDate(item.update_date)}
                      </span>
                    </div>
                  </div>
                </Card>
              </Col>
            );
          })}
        </Row>
      </div>
    </>
  );
};

export default Knowledge;
