import { useNextFetchKnowledgeList } from '@/hooks/knowledge-hooks';
import { UserOutlined } from '@ant-design/icons';
import type { CheckboxProps } from 'antd';
import { Avatar, Checkbox, Layout, List, Space, Typography } from 'antd';
import { CheckboxChangeEvent } from 'antd/es/checkbox';
import { CheckboxValueType } from 'antd/es/checkbox/Group';
import classNames from 'classnames';
import {
  Dispatch,
  SetStateAction,
  useCallback,
  useEffect,
  useMemo,
} from 'react';

import styles from './index.less';

const { Sider } = Layout;

interface IProps {
  isFirstRender: boolean;
  checkedList: string[];
  setCheckedList: Dispatch<SetStateAction<string[]>>;
}

const SearchSidebar = ({
  isFirstRender,
  checkedList,
  setCheckedList,
}: IProps) => {
  const { list, loading } = useNextFetchKnowledgeList();
  const ids = useMemo(() => list.map((x) => x.id), [list]);

  const checkAll = list.length === checkedList.length;

  const indeterminate =
    checkedList.length > 0 && checkedList.length < list.length;

  const onChange = useCallback(
    (list: CheckboxValueType[]) => {
      setCheckedList(list as string[]);
    },
    [setCheckedList],
  );

  const onCheckAllChange: CheckboxProps['onChange'] = useCallback(
    (e: CheckboxChangeEvent) => {
      setCheckedList(e.target.checked ? ids : []);
    },
    [ids, setCheckedList],
  );

  useEffect(() => {
    setCheckedList(ids);
  }, [ids, setCheckedList]);

  return (
    <Sider
      className={classNames(styles.searchSide, {
        [styles.transparentSearchSide]: isFirstRender,
      })}
      theme={'light'}
      width={240}
    >
      <Checkbox
        className={styles.modelForm}
        indeterminate={indeterminate}
        onChange={onCheckAllChange}
        checked={checkAll}
      >
        All
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
          loading={loading}
          renderItem={(item) => (
            <List.Item>
              <Checkbox value={item.id} className={styles.checkbox}>
                <Space>
                  <Avatar size={30} icon={<UserOutlined />} src={item.avatar} />
                  <Typography.Text
                    ellipsis={{ tooltip: item.name }}
                    className={styles.knowledgeName}
                  >
                    {item.name}
                  </Typography.Text>
                </Space>
              </Checkbox>
            </List.Item>
          )}
        />
      </Checkbox.Group>
    </Sider>
  );
};

export default SearchSidebar;
