import { Navigate, useLocation } from 'react-router';

// Redirect that keeps the current query string and hash, so migrated
// URLs (e.g. the data-source detail page's `?id=`) do not drop state.
export function QueryPreservingNavigate({ to }: { to: string }) {
  const location = useLocation();

  return (
    <Navigate
      replace
      to={{ pathname: to, search: location.search, hash: location.hash }}
    />
  );
}
