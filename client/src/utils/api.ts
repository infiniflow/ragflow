import config from '@/utils/config';

const host = window.location.host;

let api_host = `//mp.test.amnetapi.com/mp/v1`;


// api_host = '//mpcompany3.test.amnetapi.com/mp/v1';
let login = '//mp41.test.amnetapi.com/mp/v1/user/ssoLogin';
// sso_host = `//test120-sso.amnetapi.com`;

export { api_host };

export default {
  icp: config.COPY_RIGHT_TEXT,

  upload: `${api_host}/upload`,
  uploadZip: `${api_host}/uploadZip`,
  segment_upload: `${api_host}/uploadPopulation`,

  // 用户
  login: login,
  user: `${api_host}/user/validate`,
  getUrl: `${api_host}/requestGetUrl`,
  getAdPermits: `${api_host}/adServer/getAdPermits`,

  //子用户管理
  account_list: `${api_host}/user/getUserList`,
  create_account: `${api_host}/user/createUserAccountSso`,
  update_account: `${api_host}/user/updateUserAccountSso`,
  account_detail: `${api_host}/user/getUserDetail`,
  getUserDetail: `${api_host}/user/getUserDetail`,
  account_status: `${api_host}/user/updateAccountStatus`,
  sign_agreement: `${api_host}/user/updateUserSignAgreement`,

};
