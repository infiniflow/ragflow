

const routes = [
  {
    path: '/login',
    component: '@/pages/login',
    layout: false
  },
  {
    path: '/',
    component: '@/layouts', // 默认页面
    redirect: '/knowledge',
    // wrappers: [
    //   '@/wrappers/auth',
    // ]
  },

  {
    id: 2,
    name: '知识库',
    icon: 'home',
    auth: [3, 4, 100],
    path: '/knowledge',
    component: '@/pages/knowledge',
    pathname: 'knowledge'
  },
  {
    id: 2,
    name: '知识库',
    icon: 'home',
    auth: [3, 4, 100],
    path: '/knowledge/add/*',
    component: '@/pages/add-knowledge',
    pathname: 'knowledge',
    // routes: [{
    //   id: 3,
    //   name: '设置',
    //   icon: 'home',
    //   auth: [3, 4, 100],
    //   path: '/knowledge/add/setting',
    //   component: '@/pages/setting',
    //   pathname: "setting"
    // }, {
    //   id: 1,
    //   name: '文件',
    //   icon: 'file',
    //   auth: [3, 4, 100],
    //   path: '/knowledge/add/file',
    //   component: '@/pages/file',
    //   pathname: 'file'
    // },]
  },
  {
    id: 3,
    name: '聊天',
    icon: 'home',
    auth: [3, 4, 100],
    path: '/chat',
    component: '@/pages/chat',
    pathname: "chat"
  },
  {
    id: 3,
    name: '设置',
    icon: 'home',
    auth: [3, 4, 100],
    path: '/setting',
    component: '@/pages/setting',
    pathname: "setting"
  },
  {
    path: '/*',
    component: '@/pages/404',
    layout: false
  }

];


module.exports = routes;