import React from 'react';
import { connect } from 'umi'
import { UploadOutlined } from '@ant-design/icons';
import type { UploadProps } from 'antd';
import { Button, message, Upload } from 'antd';
import uploadService from '@/services/uploadService'


const Index = ({ kb_id, getKfList }) => {
    console.log(kb_id)
    const createRequest = async function ({ file, onSuccess, onError }) {
        const { retcode, data } = await uploadService.uploadFile(file, kb_id);
        if (retcode === 0) {
            onSuccess(data, file);

        } else {
            onError(data);
        }
        getKfList && getKfList()
    };
    const uploadProps: UploadProps = {
        customRequest: createRequest,
        showUploadList: false,
    };
    return (<Upload {...uploadProps} >
        <Button type="link">导入文件</Button>
    </Upload>)
}

export default connect(({ kFModel, settingModel, loading }) => ({ kFModel, settingModel, loading }))(Index);