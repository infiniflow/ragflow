import config from '@/utils/config';


let api_host = `/v1`;


export { api_host };

export default {
  icp: config.COPY_RIGHT_TEXT,

  upload: `${api_host}/upload`,
  uploadZip: `${api_host}/uploadZip`,
  segment_upload: `${api_host}/uploadPopulation`,

  // 用户
  login: `${api_host}/user/login`,
  register: `${api_host}/user/register`,
  setting: `${api_host}/user/setting`,
  user_info: `${api_host}/user/info`,
  tenant_info: `${api_host}/user/tenant_info`,
  user: `${api_host}/user/validate`,
  getUrl: `${api_host}/requestGetUrl`,
  getAdPermits: `${api_host}/adServer/getAdPermits`,

  //知识库管理
  kb_list: `${api_host}/kb/list`,
  create_kb: `${api_host}/kb/create`,
  update_kb: `${api_host}/kb/update`,
  rm_kb: `${api_host}/kb/rm`,
  update_account: `${api_host}/user/updateUserAccountSso`,
  account_detail: `${api_host}/user/getUserDetail`,
  getUserDetail: `${api_host}/user/getUserDetail`,
  account_status: `${api_host}/user/updateAccountStatus`,
  sign_agreement: `${api_host}/user/updateUserSignAgreement`,

};
