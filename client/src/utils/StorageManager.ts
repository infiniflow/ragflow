import { debounce } from 'lodash';
import semver from 'semver';
import merge from 'lodash/merge';
import isPlainObject from 'lodash/isPlainObject';

class StorageManager {
  constructor(store, options) {
    options = Object.assign(
      {},
      {
        lazy: true,
        namespace: 'STORAGE_MANAGER',
        version: '0.0.0'
      },
      options
    );
    this.namespace = options.namespace;
    this.isLazy = options.lazy;
    this._store = store;
    this.loaded = false;
    this.version = options.version;

    this.cache = {};
    this.initStorage();
  }

  initStorage() {
    if (!isPlainObject(this._store)) {
      throw new Error('store should be a plain object');
    }
    if (this.checkStore()) {
      this.setItem(this.namespace, this.buildData(this._store));
    }
    this.initStore();
    if (!this.isLazy) {
      this.fillCache();
    }
  }

  initStore() {
    const keys = Object.keys(this._store);
    let i = keys.length;
    while (i--) {
      this.proxy(keys[i]);
    }
  }

  proxy(key) {
    Object.defineProperty(this, key, {
      configurable: true,
      enumerable: true,
      get: () => {
        if (!this.loaded && this.isLazy) {
          this.fillCache();
        }
        return this.cache[key];
      },
      set: val => {
        if (!this.loaded && this.isLazy) {
          this.fillCache();
        }
        this.cache[key] = val;
      }
    });
  }

  observe(data) {
    if (Object.prototype.toString.call(data) !== '[object Object]') {
      return;
    }
    let keys = Object.keys(data);
    for (let i = 0; i < keys.length; i++) {
      this.defineReactive(data, keys[i], data[keys[i]]);
    }
  }

  defineReactive(data, key, val) {
    this.observe(val);
    Object.defineProperty(data, key, {
      configurable: true,
      enumerable: true,
      get: () => {
        return val;
      },
      set: newVal => {
        if (val === newVal) {
          return;
        }
        val = newVal;
        this.observe(newVal);
        this.debounceSet();
      }
    });
  }

  fillCache() {
    this.cache = merge({}, this._store, this.getItem(this.namespace).data);
    this.loaded = true;
    this.observe(this.cache);
  }

  checkStore() {
    const item = this.getItem(this.namespace);
    return !!(!item || semver.lt(item.version, this.version));
  }

  buildData(data) {
    return {
      version: this.version,
      data
    };
  }

  debounceSet() {
    return debounce(this.setItem, 200)(this.namespace, this.buildData(this.cache));
  }

  setItem(key, value) {
    window.localStorage.setItem(key, JSON.stringify(value));
  }

  getItem(key) {
    try {
      return JSON.parse(window.localStorage.getItem(key));
    } catch (e) {
      return null;
    }
  }
}
export default StorageManager;
