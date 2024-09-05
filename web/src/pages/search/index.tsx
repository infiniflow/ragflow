import { Layout } from 'antd';
import React from 'react';
import SearchSidebar from './sidebar';

const { Header, Content, Footer } = Layout;

const SearchPage = () => {
  return (
    <Layout hasSider>
      <SearchSidebar></SearchSidebar>
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
