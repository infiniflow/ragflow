import { createContext } from 'react';

const SettingContext = createContext({ setRefreshCount: () => {}, kb_id: '' });
export default SettingContext;
