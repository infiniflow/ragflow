import React from 'react';
import { connect, Dispatch } from 'umi';
import i18n from 'i18next';
import { useTranslation, Trans } from 'react-i18next'
import { Input, Modal, Form, Tag, Space } from 'antd'
import { rsaPsw } from '@/utils'
import { useEffect, useState } from 'react';
import styles from './index.less';
import type { kFModelState } from './model'
import type { settingModelState } from '@/pages/setting/model'
const { CheckableTag } = Tag;
interface kFProps {
    dispatch: Dispatch;
    kFModel: kFModelState;
    settingModel: settingModelState;
    getKfList: () => void;
    parser_id: string;
    doc_id: string;
}
const Index: React.FC<kFProps> = ({ kFModel, settingModel, dispatch, getKfList, parser_id, doc_id }) => {
    const [selectedTag, setSelectedTag] = useState('')
    const { tenantIfo = {} } = settingModel
    const { parser_ids = '' } = tenantIfo
    useEffect(() => {
        dispatch({
            type: 'settingModel/getTenantInfo',
            payload: {
            }
        });
        setSelectedTag(parser_id)
    }, [parser_id])
    const { isShowSegmentSetModal } = kFModel
    const { t } = useTranslation()
    const handleCancel = () => {
        dispatch({
            type: 'kFModel/updateState',
            payload: {
                isShowSegmentSetModal: false
            }
        });
    };
    const handleOk = () => {
        console.log(1111, selectedTag)
        dispatch({
            type: 'kFModel/document_change_parser',
            payload: {
                parser_id: selectedTag,
                doc_id
            },
            callback: () => {
                dispatch({
                    type: 'kFModel/updateState',
                    payload: {
                        isShowSegmentSetModal: false
                    }
                });
                getKfList && getKfList()
            }
        });
    };

    const handleChange = (tag: string, checked: boolean) => {
        const nextSelectedTag = checked
            ? tag
            : selectedTag;
        console.log('You are interested in: ', nextSelectedTag);
        setSelectedTag(nextSelectedTag);
    };

    return (
        <Modal title="Basic Modal" open={isShowSegmentSetModal} onOk={handleOk} onCancel={handleCancel}>
            <Space size={[0, 8]} wrap>
                <div className={styles.tags}>
                    {
                        parser_ids.split(',').map((tag: string) => {
                            return (<CheckableTag
                                key={tag}
                                checked={selectedTag === tag}
                                onChange={(checked) => handleChange(tag, checked)}
                            >
                                {tag}
                            </CheckableTag>)
                        })
                    }
                </div>
            </Space>
        </Modal >


    );
}
export default connect(({ kFModel, settingModel, loading }) => ({ kFModel, settingModel, loading }))(Index);
