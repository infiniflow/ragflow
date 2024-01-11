import { connect } from 'umi';
import { Input, Form, Button, Checkbox } from 'antd';
// import md5 from 'md5';
import styles from './index.less';
import JSEncrypt from 'jsencrypt';
import { Base64 } from 'js-base64';
import Title from 'antd/es/skeleton/Title';
import { useState, useEffect } from 'react';
// import Base64 from 'crypto-js/enc-base64';
const View = ({
  loginModel,
  dispatch,
  location,
}) => {
  const [title, setTitle] = useState('login')
  const pub = "-----BEGIN PUBLIC KEY-----MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEArq9XTUSeYr2+N1h3Afl/z8Dse/2yD0ZGrKwx+EEEcdsBLca9Ynmx3nIB5obmLlSfmskLpBo0UACBmB5rEjBp2Q2f3AG3Hjd4B+gNCG6BDaawuDlgANIhGnaTLrIqWrrcm4EMzJOnAOI1fgzJRsOOUEfaS318Eq9OVO3apEyCCt0lOQK6PuksduOjVxtltDav+guVAA068NrPYmRNabVKRNLJpL8w4D44sfth5RvZ3q9t+6RTArpEtc5sh5ChzvqPOzKGMXW83C95TxmXqpbK6olN4RevSfVjEAgCydH6HN6OhtOQEcnrU97r9H0iZOWwbw3pVrZiUkuRD1R56Wzs2wIDAQAB-----END PUBLIC KEY-----"
  const changeTitle = () => {
    setTitle((title) => title === 'login' ? 'register' : 'login')
  }
  const [form] = Form.useForm();
  const [checkNick, setCheckNick] = useState(false);

  useEffect(() => {
    form.validateFields(['nickname']);
  }, [checkNick, form]);

  const onCheck = async () => {
    try {
      const params = await form.validateFields();
      console.log('Success:', params);
      const encryptor = new JSEncrypt()
      encryptor.setPublicKey(pub)
      var rsaPassWord = encryptor.encrypt(Base64.encode(params.password))
      if (title === 'login') {
        dispatch({
          type: 'loginModel/login',
          payload: {
            email: params.email,
            password: rsaPassWord
          }
        });
      } else {
        dispatch({
          type: 'loginModel/register',
          payload: {
            nickname: params.nickname,
            email: params.email,
            password: rsaPassWord
          }
        });
      }
    } catch (errorInfo) {
      console.log('Failed:', errorInfo);
    }
  };
  const formItemLayout = {
    labelCol: { span: 4 },
    wrapperCol: { span: 8 },
  };

  const formTailLayout = {
    labelCol: { span: 4 },
    wrapperCol: { span: 8, offset: 4 },
  };
  return (
    <div className={styles.loginContainer}>
      {title === 'login' ? '登录' : '注册'}
      <div className={styles.modal}>
        <Form form={form} name="dynamic_rule" style={{ maxWidth: 600 }}>
          <Form.Item
            {...formItemLayout}
            name="email"
            label="Email"
            rules={[{ required: true, message: 'Please input your name' }]}
          >
            <Input placeholder="Please input your name" />
          </Form.Item>
          {
            title === 'register' && <Form.Item
              {...formItemLayout}
              name="nickname"
              label="Nickname"
              rules={[{ required: checkNick, message: 'Please input your nickname' }]}
            >
              <Input placeholder="Please input your nickname" />
            </Form.Item>
          }
          <Form.Item
            {...formItemLayout}
            name="password"
            label="Password"
            rules={[{ required: true, message: 'Please input your name' }]}
          >
            <Input placeholder="Please input your name" />
          </Form.Item>
          <div>   {
            title === 'login' && (<div>
              没有账号?<a onClick={changeTitle}>注册</a>
            </div>)
          }
            {
              title === 'register' && (<div>
                已有账号?<a onClick={changeTitle}>登录</a>
              </div>)
            }</div>
          <Form.Item {...formTailLayout}>
            <Button type="primary" onClick={onCheck}>
              Check
            </Button>
          </Form.Item>
        </Form>
      </div>
    </div>
  );
};

export default connect(({ loginModel, loading }) => ({ loginModel, loading }))(View);
