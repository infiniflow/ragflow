/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

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
      state: window.history.state,
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
