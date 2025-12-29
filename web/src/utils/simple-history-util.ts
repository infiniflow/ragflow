import { useNavigate } from 'react-router';

let navigateFunction: ReturnType<typeof useNavigate> | null = null;

export const useCustomNavigate = () => {
  try {
    const navigate = useNavigate();
    navigateFunction = navigate;
    return navigate;
  } catch (error) {
    console.warn('useNavigate must be used within a Router context');
    return () => {};
  }
};

export const history = {
  push: (path: string) => {
    if (navigateFunction) {
      navigateFunction(path);
    } else {
      console.warn(
        'Navigate function is not initialized. Make sure to call useCustomNavigate in your app.',
      );
    }
  },

  back: () => {
    if (navigateFunction) {
      navigateFunction(-1);
    } else {
      console.warn(
        'Navigate function is not initialized. Make sure to call useCustomNavigate in your app.',
      );
    }
  },

  replace: (path: string) => {
    if (navigateFunction) {
      navigateFunction(path, { replace: true });
    } else {
      console.warn(
        'Navigate function is not initialized. Make sure to call useCustomNavigate in your app.',
      );
    }
  },
};
