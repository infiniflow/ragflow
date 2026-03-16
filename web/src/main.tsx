import React from 'react';
import { gotoVSCode, Inspector } from 'react-dev-inspector';
import ReactDOM from 'react-dom/client';
import '../tailwind.css';
import App from './app';
import './global.less';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <Inspector keys={['alt', 'c']} onInspectElement={gotoVSCode} />
    <App />
  </React.StrictMode>,
);
