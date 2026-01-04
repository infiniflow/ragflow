class GlobalHistory {
  private listeners: Array<(location: any, action: string) => void> = [];
  state: any;

  constructor() {
    window.addEventListener('popstate', this.handlePopState);
  }

  private handlePopState = (event: PopStateEvent) => {
    const location = {
      pathname: window.location.pathname,
      search: window.location.search,
      hash: window.location.hash,
      state: event.state,
    };

    this.listeners.forEach((listener) => {
      listener(location, 'POP');
    });
  };

  push = (
    path:
      | string
      | { pathname?: string; search?: string; hash?: string; state?: any },
    state?: any,
  ) => {
    let finalPath = '';
    if (typeof path === 'string') {
      finalPath = path;
    } else {
      finalPath = path.pathname || '';
      if (path.search) finalPath += path.search;
      if (path.hash) finalPath += path.hash;
    }

    window.history.pushState(state, '', finalPath);

    const location = {
      pathname: window.location.pathname,
      search: window.location.search,
      hash: window.location.hash,
      state: state,
    };

    this.listeners.forEach((listener) => {
      listener(location, 'PUSH');
    });
  };

  replace = (
    path:
      | string
      | { pathname?: string; search?: string; hash?: string; state?: any },
    state?: any,
  ) => {
    let finalPath = '';
    if (typeof path === 'string') {
      finalPath = path;
    } else {
      finalPath = path.pathname || '';
      if (path.search) finalPath += path.search;
      if (path.hash) finalPath += path.hash;
    }

    window.history.replaceState(state, '', finalPath);

    const location = {
      pathname: window.location.pathname,
      search: window.location.search,
      hash: window.location.hash,
      state: state,
    };

    this.listeners.forEach((listener) => {
      listener(location, 'REPLACE');
    });
  };

  go = (n: number) => {
    window.history.go(n);
  };

  goBack = () => {
    window.history.back();
  };

  goForward = () => {
    window.history.forward();
  };

  listen = (callback: (location: any, action: string) => void) => {
    this.listeners.push(callback);

    return () => {
      const index = this.listeners.indexOf(callback);
      if (index !== -1) {
        this.listeners.splice(index, 1);
      }
    };
  };

  get location() {
    return {
      pathname: window.location.pathname,
      search: window.location.search,
      hash: window.location.hash,
      state: history.state,
    };
  }

  get length() {
    return window.history.length;
  }

  get action() {
    return 'POP';
  }
}

export const history = new GlobalHistory();

export const useCustomNavigate = () => {
  return history;
};
