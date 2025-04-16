// 导入 antd 组件
import { Col, Divider, Row, Spin, Typography } from 'antd';
// 导入自定义组件
import CategoryPanel from './category-panel';  // 分类面板组件
import { ConfigurationForm } from './configuration';  // 配置表单组件
// 导入自定义 hooks
import {
  useHandleChunkMethodChange,  // 处理分块方法变更的 hook
  useSelectKnowledgeDetailsLoading,  // 获取知识库详情加载状态的 hook
} from './hooks';

// 导入国际化 hook
import { useTranslate } from '@/hooks/common-hooks';
// 导入样式文件
import styles from './index.less';

// 解构 Typography 组件中的 Title
const { Title } = Typography;

// 知识库配置组件
const Configuration = () => {
  // 获取知识库详情的加载状态
  const loading = useSelectKnowledgeDetailsLoading();
  // 获取表单实例和当前选择的分块方法
  const { form, chunkMethod } = useHandleChunkMethodChange();
  // 获取国际化翻译函数，指定命名空间为 knowledgeConfiguration
  const { t } = useTranslate('knowledgeConfiguration');

  return (
    <div className={styles.configurationWrapper}>
      {/* 配置标题 */}
      <Title level={5}>
        {t('configuration', { keyPrefix: 'knowledgeDetails' })}
      </Title>
      {/* 配置说明文本 */}
      <p>{t('titleDescription')}</p>
      {/* 分隔线 */}
      <Divider></Divider>
      {/* 加载状态包装器 */}
      <Spin spinning={loading}>
        {/* 使用 Row 和 Col 进行栅格布局 */}
        <Row gutter={32}>
          {/* 左侧配置表单，占据 8 列 */}
          <Col span={8}>
            <ConfigurationForm form={form}></ConfigurationForm>
          </Col>
          {/* 右侧分类面板，占据 16 列 */}
          <Col span={16}>
            <CategoryPanel chunkMethod={chunkMethod}></CategoryPanel>
          </Col>
        </Row>
      </Spin>
    </div>
  );
};

export default Configuration;
