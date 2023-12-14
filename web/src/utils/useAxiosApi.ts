import { useAxios } from '@vueuse/integrations/useAxios';
import axios from 'axios';
import { message } from 'ant-design-vue';

// create an axios instance
const instance = axios.create({
  withCredentials: false,
  timeout: 5000,
});

// request interceptor
instance.interceptors.request.use(
  (config) => {
    // do something before request is sent
    return config;
  },
  (error) => {
    // do something with request error
    console.log(error); // for debug
    return Promise.reject(error);
  },
);

// response interceptor
instance.interceptors.response.use(
  (response) => {
    const res = response.data;
    if (res.code !== 200) {
      message.error(res.msg);
      if (res.code === 412) {
        // store.dispatch('user/userLogout');
      }
      return Promise.reject(res.msg || 'Error');
    } else {
      return res;
    }
  },
  (error) => {
    console.log(`err${error}`);
    message.error(error.message);
    return Promise.reject(error.message);
  },
);

/**
 * reactive useFetchApi
 */

export default function useAxiosApi<T>(url: string, config?: RawAxiosRequestConfig) {
  const fnConfig = config || {};
  return useAxios<T>(url, fnConfig, instance);
}
