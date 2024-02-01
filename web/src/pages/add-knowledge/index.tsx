import { Breadcrumb } from 'antd';
import { useEffect } from 'react';
import { useDispatch, useLocation, useParams, useSelector } from 'umi';
import Chunk from './components/knowledge-chunk';
import File from './components/knowledge-file';
import Search from './components/knowledge-search';
import Setting from './components/knowledge-setting';
import Siderbar from './components/knowledge-sidebar';
import styles from './index.less';

const KnowledgeAdding = () => {
  const dispatch = useDispatch();
  const kAModel = useSelector((state: any) => state.kAModel);
  const { id, doc_id } = kAModel;

  const location = useLocation();
  const params = useParams();
  const activeKey = params.module;

  const breadcrumbItems = [
    {
      title: 'Home',
    },
    {
      title: <a href="">Application Center</a>,
    },
    {
      title: <a href="">Application List</a>,
    },
    {
      title: 'An Application',
    },
  ];

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
            {activeKey === 'file' && !doc_id && <File kb_id={id} />}
            {activeKey === 'setting' && <Setting kb_id={id} />}
            {activeKey === 'search' && <Search kb_id={id} />}
            {activeKey === 'file' && !!doc_id && <Chunk doc_id={doc_id} />}
          </div>
        </div>
      </div>
    </>
  );
};

export default KnowledgeAdding;
