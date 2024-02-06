import React, { ReactNode } from 'react';
import { Inspector } from 'react-dev-inspector';

export function rootContainer(container: ReactNode) {
  return React.createElement(Inspector, null, container);
}
