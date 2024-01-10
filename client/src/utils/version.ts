/* global VERSION */
import api from '@/utils/api';
import { extend } from 'umi-request';
import { notification, Button } from 'antd';
import config from '@/utils/config';

const { get_simple_version } = api;
const host = window.location.host;
const NOTIFICATION_KEY = 'VERSION';

// npm run build shiyi version
export const LOCAL_VERSION = VERSION;

/**
 * 这里必须重新extend一个request,不然跟全局的request为同一个对象,
 * */
const request = extend({
  errorHandler: () => {} // 默认错误处理
});

let timer;
let timeLoad = () => {
  timer = setInterval(async () => {
    try {
      const { code, publishVersion } = await request(get_simple_version);
      code === 0 && publishVersion && checkVersion(publishVersion);
    } catch (e) {}
  }, 60000);
};

if (host === 'adv.martechlab.cn' || host === 'shiyi.martechlab.cn') {
  if (!config.HIDE_VERSION) {
    timeLoad();
  }
}

/***
 *
 * @param publishVersion 接口取到的version
 * @param LOCAL_VERSION  本地version，每次构建打包，webpack会根据打包命令写入版本号
 */

//检查远端版本是否跟本地一致
const checkVersion = publishVersion => {
  if (LOCAL_VERSION?.slice(0, 3) !== publishVersion?.slice(0, 3)) {
    clearInterval(timer);
    //不相等证明有弹窗提醒
    notification.info({
      key: NOTIFICATION_KEY,
      top: 50,
      duration: null,
      message: <span>发现版本更新</span>,
      onClose: () => timeLoad(),
      description: (
        <div>
          <div>发现版本更新，是否刷新页面获取；</div>
          <div style={{ color: 'orange' }}>注意！若刷新页面当前输入内容不做保存！</div>
        </div>
      ),
      btn: (
        <div>
          <Button
            style={{ marginRight: 15 }}
            onClick={() => {
              notification.close(NOTIFICATION_KEY);
              timeLoad();
            }}
          >
            取消
          </Button>{' '}
          <Button type="primary" onClick={() => history.go(0)}>
            刷新
          </Button>
        </div>
      )
    });
  }
};
