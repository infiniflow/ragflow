import { useNextFetchKnowledgeList } from '@/hooks/knowledge-hooks';
import { Checkbox, Layout, List, Typography } from 'antd';
import React, { useCallback } from 'react';

import { CheckboxValueType } from 'antd/es/checkbox/Group';
import styles from './index.less';

const { Header, Content, Footer, Sider } = Layout;

const SearchPage = () => {
  const { list } = useNextFetchKnowledgeList();

  const handleChange = useCallback((checkedValue: CheckboxValueType[]) => {
    console.log('🚀 ~ handleChange ~ args:', checkedValue);
  }, []);

  return (
    <Layout hasSider>
      <Sider className={styles.searchSide} theme={'light'}>
        <Checkbox.Group className={styles.checkGroup} onChange={handleChange}>
          <List
            bordered
            dataSource={list}
            className={styles.list}
            renderItem={(item) => (
              <List.Item>
                <Checkbox value={item.id} className={styles.checkbox}>
                  <Typography.Text
                    ellipsis={{ tooltip: item.name }}
                    className={styles.knowledgeName}
                  >
                    {item.name}
                  </Typography.Text>
                </Checkbox>
              </List.Item>
            )}
          />
        </Checkbox.Group>
      </Sider>
      <Layout style={{ marginInlineStart: 200 }}>
        <Header style={{ padding: 0 }} />
        <Content style={{ margin: '24px 16px 0', overflow: 'initial' }}>
          <div
            style={{
              padding: 24,
              textAlign: 'center',
            }}
          >
            <p>long content</p>
            {
              // indicates very long content
              Array.from({ length: 100 }, (_, index) => (
                <React.Fragment key={index}>
                  {index % 20 === 0 && index ? 'more' : '...'}
                  <br />
                </React.Fragment>
              ))
            }
          </div>
        </Content>
        <Footer style={{ textAlign: 'center' }}>
          Ant Design ©{new Date().getFullYear()} Created by Ant UED
        </Footer>
      </Layout>
    </Layout>
  );
};

export default SearchPage;
