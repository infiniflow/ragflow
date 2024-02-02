import { Breadcrumb } from 'antd';
import { useEffect, useMemo } from 'react';
import {
  useDispatch,
  useLocation,
  useNavigate,
  useParams,
  useSelector,
} from 'umi';
import Chunk from './components/knowledge-chunk';
import File from './components/knowledge-file';
import Search from './components/knowledge-search';
import Setting from './components/knowledge-setting';
import Siderbar from './components/knowledge-sidebar';
import { KnowledgeRouteKey, routeMap } from './constant';
import styles from './index.less';

const KnowledgeAdding = () => {
  const dispatch = useDispatch();
  const kAModel = useSelector((state: any) => state.kAModel);
  const navigate = useNavigate();
  const { id, doc_id } = kAModel;

  const location = useLocation();
  const params = useParams();
  const activeKey: KnowledgeRouteKey =
    (params.module as KnowledgeRouteKey) || KnowledgeRouteKey.Dataset;

  const gotoList = () => {
    navigate('/knowledge');
  };

  const breadcrumbItems = useMemo(() => {
    return [
      {
        title: <a onClick={gotoList}>Knowledge Base</a>,
      },
      {
        title: routeMap[activeKey],
      },
    ];
  }, [activeKey]);

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
  }, [location]);

  return (
    <>
      <div className={styles.container}>
        <Siderbar></Siderbar>
        <div className={styles.contentWrapper}>
          <Breadcrumb items={breadcrumbItems} />
          <div className={styles.content}>
            {activeKey === KnowledgeRouteKey.Dataset && !doc_id && (
              <File kb_id={id} />
            )}
            {activeKey === KnowledgeRouteKey.Configration && (
              <Setting kb_id={id} />
            )}
            {activeKey === KnowledgeRouteKey.Testing && <Search kb_id={id} />}
            {activeKey === KnowledgeRouteKey.Dataset && !!doc_id && (
              <Chunk doc_id={doc_id} />
            )}
          </div>
        </div>
      </div>
    </>
  );
};

export default KnowledgeAdding;
