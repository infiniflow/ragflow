import { connect, Dispatch } from 'umi';
import i18n from 'i18next';
import { useTranslation, Trans } from 'react-i18next'

import styles from './index.less';
import type { ColumnsType } from 'antd/es/table';
import { useEffect, useState, FC } from 'react';

import { RadarChartOutlined } from '@ant-design/icons';
import { ProCard } from '@ant-design/pro-components';
import { Button, Tag, Row, Col, Card } from 'antd';


interface DataType {
    key: React.Key;
    name: string;
    age: number;
    address: string;
    description: string;
}
interface ListProps {
    dispatch: Dispatch;
    settingModel: any
}
const Index: FC<ListProps> = ({ settingModel, dispatch }) => {
    const { llmInfo = {}, factoriesList, myLlm = [] } = settingModel
    const { OpenAI = [], tongyi = [] } = llmInfo
    console.log(OpenAI)
    const [collapsed, setCollapsed] = useState(true);
    const { t } = useTranslation()
    const columns: ColumnsType<DataType> = [
        { title: 'Name', dataIndex: 'name', key: 'name' },
        { title: 'Age', dataIndex: 'age', key: 'age' },
        {
            title: 'Action',
            dataIndex: '',
            key: 'x',
            render: () => <a>Delete</a>,
        },
    ];
    useEffect(() => {
        dispatch({
            type: 'settingModel/factories_list',
            payload: {
            },
        });
        dispatch({
            type: 'settingModel/llm_list',
            payload: {
            },
        });
        dispatch({
            type: 'settingModel/my_llm',
            payload: {
            },
        });

    }, [])
    const data: DataType[] = [
        {
            key: 1,
            name: 'John Brown',
            age: 32,
            address: 'New York No. 1 Lake Park',
            description: 'My name is John Brown, I am 32 years old, living in New York No. 1 Lake Park.',
        },
        {
            key: 2,
            name: 'Jim Green',
            age: 42,
            address: 'London No. 1 Lake Park',
            description: 'My name is Jim Green, I am 42 years old, living in London No. 1 Lake Park.',
        },
        {
            key: 3,
            name: 'Not Expandable',
            age: 29,
            address: 'Jiangsu No. 1 Lake Park',
            description: 'This not expandable',
        },
        {
            key: 4,
            name: 'Joe Black',
            age: 32,
            address: 'Sydney No. 1 Lake Park',
            description: 'My name is Joe Black, I am 32 years old, living in Sydney No. 1 Lake Park.',
        },
    ];

    return (
        <div
            className={styles.list}
            style={{
                display: 'flex',
                flexDirection: 'column',
                padding: 24,
                gap: 12,
            }}
        >
            {
                myLlm.map((item: any) => {
                    return (<ProCard
                        key={item.llm_factory}
                        // title={<div>可折叠-图标自定义</div>}
                        collapsibleIconRender={({
                            collapsed: buildInCollapsed,
                        }: {
                            collapsed: boolean;
                        }) => {
                            return (<div>
                                <h3><RadarChartOutlined />{item.llm_factory}</h3>
                                <div>{item.tags.split(',').map((d: string) => {
                                    return <Tag key={d}>{d}</Tag>
                                })}</div>
                                {
                                    buildInCollapsed ? <span>显示{OpenAI.length}个模型</span> : <span>收起{OpenAI.length}个模型 </span>
                                }
                            </div>)
                        }}
                        extra={
                            <Button
                                size="small"
                                type='link'
                                onClick={(e) => {
                                    e.stopPropagation();
                                    dispatch({
                                        type: 'settingModel/updateState',
                                        payload: {
                                            llm_factory: item.llm_factory,
                                            isShowSAKModal: true
                                        }
                                    });
                                }}
                            >
                                设置
                            </Button>
                        }
                        style={{ marginBlockStart: 16 }}
                        headerBordered
                        collapsible
                        defaultCollapsed
                    >
                        {/* <ul>
                            {OpenAI.map(item => {
                                return <li key={item.llm_name}>
                                    <span>{item.llm_name}</span>
                                    <span className={styles[item.available ? 'statusAvailable' : 'statusDisaabled']}>
                                    </span>
                                </li>
                            })}
                        </ul> */}
                    </ProCard>)
                })
            }



            <Row gutter={{ xs: 8, sm: 16, md: 24, lg: 32 }}>
                {
                    factoriesList.map((item: any) => {
                        return (<Col key={item.name} xs={24} sm={12} md={8} lg={6}>
                            <Card title={item.name} bordered={false} extra={
                                <Button
                                    size="small"
                                    type='link'
                                    onClick={(e) => {
                                        e.stopPropagation();
                                        dispatch({
                                            type: 'settingModel/updateState',
                                            payload: {
                                                llm_factory: item.name,
                                                isShowSAKModal: true
                                            }
                                        });
                                    }}
                                >
                                    设置
                                </Button>
                            }>

                                <div>
                                    {
                                        item.tags.split(',').map((d: string) => {
                                            return <Tag key={d}>{d}</Tag>
                                        })
                                    }
                                </div>
                            </Card>
                        </Col>)
                    })
                }
            </Row>
        </div>
    );
}
export default connect(({ settingModel, loading }) => ({ settingModel, loading }))(Index);
