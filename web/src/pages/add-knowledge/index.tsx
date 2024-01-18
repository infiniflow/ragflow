import { connect, useNavigate, useLocation, Dispatch } from 'umi'
import React, { useState, useEffect } from 'react';
import type { MenuProps } from 'antd';
import { Menu } from 'antd';
import {
    ToolOutlined,
    BarsOutlined,
    SearchOutlined
} from '@ant-design/icons';
import File from './components/knowledge-file'
import Setting from './components/knowledge-setting'
import Search from './components/knowledge-search'
import Chunk from './components/knowledge-chunk'
import styles from './index.less'
import { getWidth } from '@/utils'
import { kAModelState } from './model'


interface kAProps {
    dispatch: Dispatch;
    kAModel: kAModelState;
}
const Index: React.FC<kAProps> = ({ kAModel, dispatch }) => {
    const [collapsed, setCollapsed] = useState(false);
    const { id, activeKey, doc_id } = kAModel
    const [windowWidth, setWindowWidth] = useState(getWidth());
    let navigate = useNavigate();
    const location = useLocation();
    // 标记一下
    console.log(doc_id, '>>>>>>>>>>>>>doc_id')
    useEffect(() => {
        const widthSize = () => {
            const width = getWidth()
            console.log(width)

            setWindowWidth(width);
        };
        window.addEventListener("resize", widthSize);
        return () => {
            window.removeEventListener("resize", widthSize);
        };
    }, []);
    useEffect(() => {
        console.log(location)
        const search = location.search.slice(1)
        const map = search.split('&').reduce((obj, cur) => {
            const [key, value] = cur.split('=')
            obj[key] = value
            return obj
        }, {})
        dispatch({
            type: 'kAModel/updateState',
            payload: {
                doc_id: undefined,
                ...map,

            }
        });
    }, [location])
    useEffect(() => {
        if (windowWidth.width > 957) {
            setCollapsed(false)
        } else {
            setCollapsed(true)
        }
    }, [windowWidth.width])
    type MenuItem = Required<MenuProps>['items'][number];

    function getItem(
        label: React.ReactNode,
        key: React.Key,
        icon?: React.ReactNode,
        children?: MenuItem[],
        type?: 'group',
    ): MenuItem {
        return {
            key,
            icon,
            children,
            label,
            type,
        } as MenuItem;
    }
    const items: MenuItem[] = [
        getItem('配置', 'setting', <ToolOutlined />),
        getItem('知识库', 'file', <BarsOutlined />),
        getItem('搜索测试', 'search', <SearchOutlined />),
    ];
    const handleSelect: MenuProps['onSelect'] = (e) => {
        navigate(`/knowledge/add/setting?activeKey=${e.key}&id=${id}`);
    }
    return (
        <>
            <div className={styles.container}>
                <div className={styles.menu}>
                    <Menu
                        selectedKeys={[activeKey]}
                        mode="inline"
                        className={windowWidth.width > 957 ? styles.defaultWidth : styles.minWidth}
                        inlineCollapsed={collapsed}
                        items={items}
                        onSelect={handleSelect}
                    />
                </div>
                <div className={styles.content}>
                    {activeKey === 'file' && !doc_id && <File kb_id={id} />}
                    {activeKey === 'setting' && <Setting kb_id={id} />}
                    {activeKey === 'search' && <Search />}
                    {activeKey === 'file' && !!doc_id && <Chunk doc_id={doc_id} />}

                </div>
            </div>
        </>
    );
};

export default connect(({ kAModel, loading }) => ({ kAModel, loading }))(Index);