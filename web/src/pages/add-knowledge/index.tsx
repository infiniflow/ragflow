import { useKnowledgeBaseId } from '@/hooks/knowledgeHook';
import {
  useNavigateWithFromState,
  useSecondPathName,
  useThirdPathName,
} from '@/hooks/routeHook';
import { Breadcrumb } from 'antd';
import { ItemType } from 'antd/es/breadcrumb/Breadcrumb';
import { useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, Outlet, useDispatch, useLocation } from 'umi';
import Siderbar from './components/knowledge-sidebar';
import {
  KnowledgeDatasetRouteKey,
  KnowledgeRouteKey,
  datasetRouteMap,
  routeMap,
} from './constant';
import styles from './index.less';

const KnowledgeAdding = () => {
  const dispatch = useDispatch();
  const knowledgeBaseId = useKnowledgeBaseId();

  const { t } = useTranslation();
  const location = useLocation();
  const activeKey: KnowledgeRouteKey =
    (useSecondPathName() as KnowledgeRouteKey) || KnowledgeRouteKey.Dataset;

  const datasetActiveKey: KnowledgeDatasetRouteKey =
    useThirdPathName() as KnowledgeDatasetRouteKey;

  const gotoList = useNavigateWithFromState();

  const breadcrumbItems: ItemType[] = useMemo(() => {
    const items: ItemType[] = [
      {
        title: (
          <a onClick={() => gotoList('/knowledge')}>
            {t('header.knowledgeBase')}
          </a>
        ),
      },
      {
        title: datasetActiveKey ? (
          <Link
            to={`/knowledge/${KnowledgeRouteKey.Dataset}?id=${knowledgeBaseId}`}
          >
            {t(`knowledgeDetails.${activeKey}`)}
          </Link>
        ) : (
          routeMap[activeKey]
        ),
      },
    ];

    if (datasetActiveKey) {
      items.push({
        title: datasetRouteMap[datasetActiveKey],
      });
    }

    return items;
  }, [activeKey, datasetActiveKey, gotoList, knowledgeBaseId, t]);

  useEffect(() => {
    const search: string = location.search.slice(1);
    const map = search.split('&').reduce<Record<string, string>>((obj, cur) => {
      const [key, value] = cur.split('=');
      obj[key] = value;
      return obj;
    }, {});

    dispatch({
      type: 'kAModel/updateState',
      payload: {
        doc_id: undefined,
        ...map,
      },
    });
  }, [location, dispatch]);

  return (
    <>
      <div className={styles.container}>
        <Siderbar></Siderbar>
        <div className={styles.contentWrapper}>
          <Breadcrumb items={breadcrumbItems} />
          <div className={styles.content}>
            <Outlet></Outlet>
          </div>
        </div>
      </div>
    </>
  );
};

export default KnowledgeAdding;
