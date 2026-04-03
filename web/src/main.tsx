import React from 'react';
import ReactDOM from 'react-dom/client';
import '../tailwind.css';
import App from './app';
import './global.less';
import { initLanguage } from './locales/config';

initLanguage().then(async () => {
  const rootEl = document.getElementById('root')!;
  const enableDevInspector =
    import.meta.env.DEV && import.meta.env.VITE_ENABLE_DEV_INSPECTOR === 'true';

  if (enableDevInspector) {
    const { gotoVSCode, Inspector } = await import('react-dev-inspector');
    ReactDOM.createRoot(rootEl).render(
      <React.StrictMode>
        <Inspector keys={['alt', 'c']} onInspectElement={gotoVSCode} />
        <App />
      </React.StrictMode>,
    );
    return;
  }

  ReactDOM.createRoot(rootEl).render(
    <React.StrictMode>
      <App />
    </React.StrictMode>,
  );
});
