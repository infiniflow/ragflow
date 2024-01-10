import React, { useState } from 'react';
import type { RadioChangeEvent } from 'antd';
import { Radio, Space, Tabs } from 'antd';
import {
    ToolOutlined,
    BarsOutlined,
    SearchOutlined
} from '@ant-design/icons';
import File from './components/knowledge-file'
import Setting from './components/knowledge-setting'
import Search from './components/knowledge-search'
import styles from './index.less'


const App: React.FC = () => {
    type keyType = 'setting' | 'file' | 'search'
    const [activeKey, setActiveKey] = useState<keyType>('file')
    // type tab = { label: string, icon: Element, tag: string }
    const tabs = [{ label: '配置', icon: <ToolOutlined />, tag: 'setting' }, { label: '知识库', icon: <BarsOutlined />, tag: 'file' }, { label: '搜索测试', icon: <SearchOutlined />, tag: 'search' }]

    const onTabClick = (activeKey: keyType) => {
        setActiveKey(activeKey)
    }
    // type stringKey = Record<string, Element>

    const mapComponent = {
        file: <File />,
        setting: <Setting />,
        search: <Search />
    }
    return (
        <>
            <Tabs
                tabPosition='left'
                activeKey={activeKey}
                onTabClick={(activeKey: keyType, e: KeyboardEvent<Element> | MouseEvent<Element, MouseEvent>) => { onTabClick(activeKey) }}
                className={styles.tab}
                items={tabs.map((item) => {
                    return {
                        label: item.label,
                        icon: item.icon,
                        key: item.tag,
                        children: mapComponent[activeKey] as Element,
                    };
                })}
            />
        </>
    );
};

export default App;