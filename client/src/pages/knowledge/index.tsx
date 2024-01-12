import React, { useEffect, useState, } from 'react';
import { useNavigate, connect } from 'umi'
import { Card, List, Popconfirm, message, FloatButton, Row, Col } from 'antd';
import { MinusSquareOutlined, DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import styles from './index.less'
import { formatDate } from '@/utils/date'

const dd = [{
  title: 'Title 4',
  text: '4',
  des: '111'
}]
const Index: React.FC = ({ knowledgeModel, dispatch }) => {
  const navigate = useNavigate()
  // const [datas, setDatas] = useState(data)
  const { data } = knowledgeModel
  const confirm = (id) => {
    dispatch({
      type: 'knowledgeModel/rmKb',
      payload: {
        kb_id: id
      },
      callback: () => {
        dispatch({
          type: 'knowledgeModel/getList',
          payload: {

          }
        });
      }
    });
  };
  const handleAddKnowledge = () => {
    navigate(`add/setting?activeKey=setting`);
  }
  const handleEditKnowledge = (id: string) => {
    navigate(`add/setting?activeKey=file&id=${id}`);
  }
  useEffect(() => {
    dispatch({
      type: 'knowledgeModel/getList',
      payload: {

      }
    });
  }, [])
  return (<>
    <div className={styles.knowledge}>
      <FloatButton onClick={handleAddKnowledge} icon={<PlusOutlined />} type="primary" style={{ right: 24, top: 100 }} />
      <Row gutter={{ xs: 8, sm: 16, md: 24, lg: 32 }}>
        {
          data.map((item, index) => {
            return (<Col className="gutter-row" key={item.title} xs={24} sm={12} md={8} lg={6}>
              <Card className={styles.card}
                onClick={() => { handleEditKnowledge(item.id) }}
              >
                <div className={styles.container}>
                  <div className={styles.content}>
                    <span className={styles.context}>
                      {item.name}
                    </span>
                    <span className={styles.delete}>
                      <Popconfirm
                        title="Delete the task"
                        description="Are you sure to delete this task?"
                        onConfirm={() => { confirm(item.id) }}
                        okText="Yes"
                        cancelText="No"
                      >
                        <DeleteOutlined />
                      </Popconfirm>

                    </span>
                  </div>
                  <div className={styles.footer}>
                    <span className={styles.text}>
                      <MinusSquareOutlined />{item.doc_num}文档
                    </span>
                    <span className={styles.text}>
                      <MinusSquareOutlined />{item.chunk_num}个
                    </span>
                    <span className={styles.text}>
                      <MinusSquareOutlined />{item.token_num}千字符
                    </span>
                    <span style={{ float: 'right' }}>
                      {formatDate(item.update_date)}
                    </span>
                  </div>

                </div>
              </Card>
            </Col>)
          })
        }
      </Row>
    </div>
  </>
  )
};

export default connect(({ knowledgeModel, loading }) => ({ knowledgeModel, loading }))(Index);