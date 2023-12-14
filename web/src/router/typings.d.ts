import 'vue-router';

declare module 'vue-router' {
  interface RouteMeta {
    icon?: string | VNode // 菜单的icon
    type?: string // 有 children 的菜单的组件类型 可选值 'group'
    title?: string // 自定义菜单的国际化 key，如果没有则返回自身
    authority?: string | string[] // 内建授权信息
    target?: TargetType // 打开目标位置 '_blank' | '_self' | null | undefined
    hideChildInMenu?: boolean // 在菜单中隐藏子节点
    hideInMenu?: boolean // 在菜单中隐藏自己和子节点
    disabled?: boolean // disable 菜单选项
    flatMenu?: boolean // 隐藏自己，并且将子节点提升到与自己平级
    ignoreCache?: boolean // 是否忽略 tab cache
    noAffix?: boolean // 是否在tab中显示
  }
}
