import React, { useMemo, useState, useEffect } from 'react';
import type { MenuProps } from 'antd';
import { Radio, Space, Tabs, Menu } from 'antd';
import {
    ToolOutlined,
    BarsOutlined,
    SearchOutlined
} from '@ant-design/icons';
import File from './components/knowledge-file'
import Setting from './components/knowledge-setting'
import Search from './components/knowledge-search'
import styles from './index.less'
import { getWidth } from '@/utils'


const App: React.FC = () => {
    const [activeKey, setActiveKey] = useState<string>('file')
    const [collapsed, setCollapsed] = useState(false);
    const [windowWidth, setWindowWidth] = useState(getWidth());

    // 标记一下
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
        setActiveKey(e.key)
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
                    {activeKey === 'file' && <File />}
                    {activeKey === 'setting' && <Setting />}
                    {activeKey === 'search' && <Search />}
                </div>
            </div>
        </>
    );
};

export default App;