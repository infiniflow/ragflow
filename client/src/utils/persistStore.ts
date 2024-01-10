import StorageManager from './StorageManager';

// 定义初始数据结构，发生改变时必须升级版本号否则会导致数据不一致
const store = {
  token: '',
  domainPrefix: '',
  userInfo: {},
  customColumns: {
    campaignTable: [],
    adUnitTable: [],
    creativeTable: [],
    vendorTable: [],
    dashboardWapper: [],
    toutiaoVendorTable: [],
    toutiaoCampaignTable: [],
    toutiaoAdUnitTable: [],
    toutiaoCreativeTable: [],
    materialVideosTable: [],
    tencentCampaignTable: [],
    tencentCreativeTable: [],
    tencentAdUnitTable: [],
    tencentVendorTable: []
  },
  vendorAuth: [],
  warningRuleIds: [],
  warningDate: '',
  ksOldCreateTimer: '',
  ttOldCreateTimer: '',
  gdtCreateTimer: ''
};

// 数据结构改变时改变
const version = '1.1.1';

export default new StorageManager(store, { version });
