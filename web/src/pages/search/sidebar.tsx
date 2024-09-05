import { useNextFetchKnowledgeList } from '@/hooks/knowledge-hooks';
import type { CheckboxProps } from 'antd';
import { Checkbox, Layout, List, Typography } from 'antd';
import { CheckboxValueType } from 'antd/es/checkbox/Group';
import { useCallback, useMemo, useState } from 'react';

import { CheckboxChangeEvent } from 'antd/es/checkbox';
import styles from './index.less';

const { Sider } = Layout;

const SearchSidebar = () => {
  const { list } = useNextFetchKnowledgeList();
  const ids = useMemo(() => list.map((x) => x.id), [list]);

  const [checkedList, setCheckedList] = useState<string[]>(ids);

  const checkAll = list.length === checkedList.length;

  const indeterminate =
    checkedList.length > 0 && checkedList.length < list.length;

  const onChange = useCallback((list: CheckboxValueType[]) => {
    setCheckedList(list as string[]);
  }, []);

  const onCheckAllChange: CheckboxProps['onChange'] = useCallback(
    (e: CheckboxChangeEvent) => {
      setCheckedList(e.target.checked ? ids : []);
    },
    [ids],
  );

  return (
    <Sider className={styles.searchSide} theme={'light'} width={260}>
      <Checkbox
        className={styles.modelForm}
        indeterminate={indeterminate}
        onChange={onCheckAllChange}
        checked={checkAll}
      >
        Check all
      </Checkbox>
      <Checkbox.Group
        className={styles.checkGroup}
        onChange={onChange}
        value={checkedList}
      >
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
  );
};

export default SearchSidebar;
