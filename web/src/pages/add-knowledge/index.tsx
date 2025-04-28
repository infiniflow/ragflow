import { useKnowledgeBaseId } from '@/hooks/knowledge-hooks';
import {
  useNavigateWithFromState,
  useSecondPathName,
  useThirdPathName,
} from '@/hooks/route-hook';
import { Breadcrumb } from 'antd';
import { ItemType } from 'antd/es/breadcrumb/Breadcrumb';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, Outlet } from 'umi';
import Siderbar from './components/knowledge-sidebar';
import { KnowledgeDatasetRouteKey, KnowledgeRouteKey } from './constant';
import styles from './index.less';

// 知识库添加页面组件
const KnowledgeAdding = () => {
  // 获取知识库ID
  const knowledgeBaseId = useKnowledgeBaseId();
  
  // 获取国际化翻译函数
  const { t } = useTranslation();
  
  // 获取当前二级路由路径，默认为Dataset
  const activeKey: KnowledgeRouteKey =
    (useSecondPathName() as KnowledgeRouteKey) || KnowledgeRouteKey.Dataset;

  // 获取当前三级路由路径
  const datasetActiveKey: KnowledgeDatasetRouteKey =
    useThirdPathName() as KnowledgeDatasetRouteKey;

  // 获取带状态的导航函数
  const gotoList = useNavigateWithFromState();

  // 使用useMemo优化面包屑导航项的计算
  const breadcrumbItems: ItemType[] = useMemo(() => {
    const items: ItemType[] = [
      // 第一级：知识库列表链接
      {
        title: (
          <a onClick={() => gotoList('/knowledge')}>
            {t('header.knowledgeBase')}
          </a>
        ),
      },
      // 第二级：当前激活的路由（数据集/设置等）
      {
        title: datasetActiveKey ? (
          // 如果存在三级路由，则显示为可点击的链接
          <Link
            to={`/knowledge/${KnowledgeRouteKey.Dataset}?id=${knowledgeBaseId}`}
          >
            {t(`knowledgeDetails.${activeKey}`)}
          </Link>
        ) : (
          // 否则显示为普通文本
          t(`knowledgeDetails.${activeKey}`)
        ),
      },
    ];

    // 如果存在三级路由，添加第三级面包屑
    if (datasetActiveKey) {
      items.push({
        title: t(`knowledgeDetails.${datasetActiveKey}`),
      });
    }

    return items;
  }, [activeKey, datasetActiveKey, gotoList, knowledgeBaseId, t]);

  // 渲染页面布局
  return (
    <>
      <div className={styles.container}>
        {/* 左侧边栏 */}
        <Siderbar></Siderbar>
        <div className={styles.contentWrapper}>
          {/* 面包屑导航 */}
          <Breadcrumb items={breadcrumbItems} />
          {/* 内容区域，使用Outlet渲染子路由内容 */}
          <div className={styles.content}>
            <Outlet></Outlet>
          </div>
        </div>
      </div>
    </>
  );
};

export default KnowledgeAdding;
