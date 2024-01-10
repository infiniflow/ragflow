
import i18n from 'i18next';
import { Button } from 'antd'
import { useTranslation, Trans } from 'react-i18next'

function Index() {
    const changeLang = (val: string) => { // 改变状态里的 语言 进行切换
        i18n.changeLanguage(val);
    }
    const { t } = useTranslation()
    return (
        <div>
            <div>
                <Button type="primary" onClick={() => i18n.changeLanguage(i18n.language == 'en' ? 'zh' : 'en')}>{t('setting.btn')}</Button>

            </div>
        </div>
    );
}
export default Index;
