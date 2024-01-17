import { connect, Dispatch } from 'umi';
import i18n from 'i18next';
import { useTranslation, Trans } from 'react-i18next'
import { Button, Input, Modal, Form, FloatButton, Table } from 'antd'


import styles from './index.less';
import CPwModal from './CPwModal'
import SAKModal from './SAKModal'
import TntModal from './TntModal'
import SSModal from './SSModal'
import List from './List'
import { useEffect, useState, FC } from 'react';
interface CPwModalProps {
    dispatch: Dispatch;
    settingModel: any
}
const Index: FC<CPwModalProps> = ({ settingModel, dispatch }) => {
    // const [llm_factory, set_llm_factory] = useState('')
    const { t } = useTranslation()
    const userInfo = JSON.parse(localStorage.getItem('userInfo') || '')
    const changeLang = (val: string) => { // 改变状态里的 语言 进行切换
        i18n.changeLanguage(val);
    }
    useEffect(() => {
        dispatch({
            type: 'settingModel/getTenantInfo',
            payload: {
            }
        });
    }, [])
    const showCPwModal = () => {
        dispatch({
            type: 'settingModel/updateState',
            payload: {
                isShowPSwModal: true
            }
        });
    };
    const showTntModal = () => {
        dispatch({
            type: 'settingModel/updateState',
            payload: {
                isShowTntModal: true
            }
        });
    };
    const showSSModal = () => {
        dispatch({
            type: 'settingModel/updateState',
            payload: {
                isShowSSModal: true
            }
        });
        // dispatch({
        //     type: 'settingModel/getTenantInfo',
        //     payload: {
        //     }
        // });
    };
    return (
        <div className={styles.settingPage}>
            <div className={styles.avatar}>
                <img style={{ width: 50, marginRight: 5 }} src="https://os.alipayobjects.com/rmsportal/QBnOOoLaAfKPirc.png" alt="" />
                <div>
                    <div>账号：{userInfo.name}</div>
                    <div><span>密码：******</span><Button type='link' onClick={showCPwModal}>修改密码</Button></div>

                </div>
            </div >
            <div>
                <Button type="link" onClick={showTntModal}>
                    租户
                </Button>
                <Button type="link" onClick={showSSModal}>
                    系统模型设置
                </Button>
                <List />
            </div>
            <CPwModal />
            <SAKModal />
            <SSModal />
            <TntModal />
            <FloatButton shape='square' description={t('setting.btn')} onClick={() => i18n.changeLanguage(i18n.language == 'en' ? 'zh' : 'en')} type="default" style={{ right: 94, fontSize: 14 }} />
        </div >


    );
}
export default connect(({ settingModel, loading }) => ({ settingModel, loading }))(Index);
